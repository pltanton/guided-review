package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/pltanton/guided-review/internal/plan"
	"github.com/pltanton/guided-review/internal/state"
)

var errGate = errors.New("gate not passed")

func cmdStatus(ctx context.Context, e env, args []string) error {
	fs := e.flags("status")
	gate := fs.Bool("gate", false, "exit non-zero unless every step is reviewed")
	if err := fs.Parse(args); err != nil {
		return err
	}
	s, r, err := loadReview(ctx, e.dir)
	if err != nil {
		return err
	}
	printHeader(e, s, r)
	if path := s.exported(r.ID); path != "" {
		e.printf("export: %s\n", path)
	}
	if len(r.Steps) == 0 {
		e.println("no plan yet: pipe a plan to gr plan set")
		if *gate {
			return errGate
		}
		return nil
	}
	e.println()
	for i, st := range r.Steps {
		if st.Chapter != "" && (i == 0 || r.Steps[i-1].Chapter != st.Chapter) {
			e.printf("  %s\n", st.Chapter)
		}
		cursor, hotspot := " ", ""
		if st.ID == r.Current {
			cursor = ">"
		}
		if len(st.Hotspots) > 0 {
			hotspot = " ⚑"
		}
		e.printf("%s %s %s  %s%s\n", cursor, st.Status.Glyph(), st.ID, st.Title, hotspot)
	}
	cov := plan.CoverageOf(r)
	e.printf("\ncore %d/%d reviewed (%d skipped, %d stale), hotspots %d/%d, "+
		"boilerplate %d files, generated %d files not reviewed\n",
		cov.Done+cov.Skipped, cov.Total, cov.Skipped, cov.Stale,
		cov.HotspotsReviewed, cov.Hotspots, cov.Boilerplate, cov.Generated)
	e.printf("comments: %s\n", severityCounts(r, ", "))
	if pending := plan.Gate(r); len(pending) > 0 {
		e.printf("gate: not passed, pending: %s\n", strings.Join(pending, " "))
		if *gate {
			return errGate
		}
		return nil
	}
	e.println("gate: passed")
	return nil
}

func severityCounts(r *state.Review, sep string) string {
	counts := map[state.Severity]int{}
	for _, c := range r.Comments {
		counts[c.Severity]++
	}
	parts := make([]string, len(state.Severities))
	for i, sev := range state.Severities {
		parts[i] = fmt.Sprintf("%d %s", counts[sev], sev)
	}
	return strings.Join(parts, sep)
}

func cmdHunks(ctx context.Context, e env, _ []string) error {
	s, r, err := loadReview(ctx, e.dir)
	if err != nil {
		return err
	}
	files, err := s.reviewDiff(ctx, r)
	if err != nil {
		return err
	}
	printHunks(e, r, files)
	return nil
}

func cmdPlan(ctx context.Context, e env, args []string) error {
	if len(args) == 0 || args[0] != "set" {
		return errors.New("usage: gr plan set [-f FILE] (repeat -f to join parts)")
	}
	fs := e.flags("plan set")
	var paths []string
	addPath := func(v string) error {
		paths = append(paths, v)
		return nil
	}
	fs.Func("f", "plan file, repeat to join parts in order (default: stdin)", addPath)
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if len(paths) == 0 {
		paths = []string{"-"}
	}
	var p plan.Plan
	for _, path := range paths {
		data, err := readInput(e, path)
		if err != nil {
			return err
		}
		part, err := plan.Parse(data)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		p.Summary = cmp.Or(p.Summary, part.Summary)
		p.Boilerplate = append(p.Boilerplate, part.Boilerplate...)
		p.Steps = append(p.Steps, part.Steps...)
	}
	s, r, err := loadReview(ctx, e.dir)
	if err != nil {
		return err
	}
	files, err := s.reviewDiff(ctx, r)
	if err != nil {
		return err
	}
	if errs := plan.Validate(p, r, files); len(errs) > 0 {
		msgs := make([]string, len(errs))
		for i, err := range errs {
			msgs[i] = err.Error()
		}
		return fmt.Errorf("plan rejected:\n  - %s", strings.Join(msgs, "\n  - "))
	}
	plan.Apply(r, p)
	r.Progress = nil
	announce(r, r.Step(r.Current))
	if err := s.store.Save(r); err != nil {
		return err
	}
	e.printf("plan accepted: %d steps", len(r.Steps))
	if len(r.Carried) > 0 {
		e.printf(", the last %d carried from earlier rounds", len(r.Carried))
	}
	e.printf("\n\n")
	printStep(e, r, r.Step(r.Current))
	return nil
}

func readInput(e env, path string) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(e.stdin)
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(e.dir, path)
	}
	return os.ReadFile(path)
}

func cmdStep(ctx context.Context, e env, args []string) error {
	if len(args) == 0 {
		args = []string{"show"}
	}
	s, r, err := loadReview(ctx, e.dir)
	if err != nil {
		return err
	}
	if len(r.Steps) == 0 {
		return errors.New("no plan yet: pipe a plan to gr plan set")
	}
	var st *state.Step
	switch args[0] {
	case "show":
		id := r.Current
		if len(args) > 1 {
			id = args[1]
		}
		if st = r.Step(id); st == nil {
			return fmt.Errorf("no step %q", id)
		}
		printStep(e, r, st)
		return nil
	case "next":
		st, err = plan.Next(r)
	case "skip":
		fs := e.flags("step skip")
		reason := fs.String("reason", "", "why the step is skipped (required)")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		st, err = plan.Skip(r, *reason)
	case "goto":
		if len(args) < 2 {
			return errors.New("usage: gr step goto ID")
		}
		err = plan.Goto(r, args[1])
		st = r.Step(r.Current)
	default:
		return fmt.Errorf("unknown step command %q", args[0])
	}
	if err != nil && !errors.Is(err, plan.ErrDone) {
		return err
	}
	announce(r, st)
	if err := s.store.Save(r); err != nil {
		return err
	}
	if st == nil {
		e.println("all steps reviewed: run gr status")
		return nil
	}
	printStep(e, r, st)
	return nil
}

func cmdComment(ctx context.Context, e env, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: gr comment add|list|edit|resolve|delete")
	}
	s, r, err := loadReview(ctx, e.dir)
	if err != nil {
		return err
	}
	var msg string
	switch args[0] {
	case "list":
		for _, c := range r.Comments {
			status := ""
			if c.Resolved {
				status = "  [resolved]"
			}
			e.printf(
				"#%d %s %s %s:%s  %s%s\n",
				c.ID,
				c.Severity,
				c.Step,
				c.File,
				c.Lines,
				c.Body,
				status,
			)
			if c.Suggestion != "" {
				e.printf("   suggestion:\n%s\n", indent(c.Suggestion, "     "))
			}
		}
		return nil
	case "edit", "resolve", "delete":
		if len(args) < 2 {
			return fmt.Errorf("usage: gr comment %s ID", args[0])
		}
		id, err := parseID(args[1])
		if err != nil {
			return err
		}
		if args[0] == "delete" {
			restored, err := plan.DeleteComment(r, id)
			if err != nil {
				return err
			}
			msg = fmt.Sprintf("comment #%d deleted", id)
			if len(restored) > 0 {
				msg += "\nback to pending: " + strings.Join(restored, " ")
			}
			break
		}
		if args[0] == "resolve" {
			if err := plan.ResolveComment(r, id); err != nil {
				return err
			}
			for i := range r.Comments {
				if r.Comments[i].ID == id {
					r.Comments[i].ResolvedRound = max(r.Round, 1)
				}
			}
			msg = fmt.Sprintf("comment #%d resolved", id)
			break
		}
		fs := e.flags("comment edit")
		severity := fs.String("severity", "", "new severity (default: keep)")
		if err := fs.Parse(args[2:]); err != nil {
			return err
		}
		body := strings.Join(fs.Args(), " ")
		if err := plan.EditComment(r, id, body, state.Severity(*severity)); err != nil {
			return err
		}
		msg = fmt.Sprintf("comment #%d updated", id)
	case "add":
		fs := e.flags("comment add")
		file := fs.String("file", "", "file path as in the diff")
		lines := fs.String("lines", "", "new-file lines, N or N-M")
		severity := fs.String("severity", "", "blocker|major|minor|nit")
		step := fs.String("step", "", "step id (default: current)")
		suggestion := fs.String("suggestion", "", "replacement text for the lines")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		c, imp, err := plan.AddComment(r, state.Comment{
			Step:       *step,
			File:       *file,
			Lines:      *lines,
			Severity:   state.Severity(*severity),
			Body:       strings.Join(fs.Args(), " "),
			Suggestion: *suggestion,
		})
		if err != nil {
			return err
		}
		msg = fmt.Sprintf("comment #%d %s %s:%s", c.ID, c.Severity, c.File, c.Lines)
		if len(imp.Stale) > 0 {
			msg += "\nstale (depend on a blocked step): " + strings.Join(imp.Stale, " ")
		}
		if len(imp.MayChange) > 0 {
			msg += "\nmay change: " + strings.Join(imp.MayChange, " ")
		}
	default:
		return fmt.Errorf("unknown comment command %q", args[0])
	}
	if err := s.store.Save(r); err != nil {
		return err
	}
	e.println(msg)
	return nil
}

func cmdNote(ctx context.Context, e env, args []string) error {
	if len(args) == 0 || args[0] != "add" && args[0] != "detail" {
		return errors.New("usage: gr note add --file F --line N [--kind note|spec] [--step ID] " +
			"TEXT | gr note detail --file F --line N [--step ID] TEXT|-")
	}
	fs := e.flags("note " + args[0])
	file := fs.String("file", "", "file path as in the diff")
	line := fs.Int("line", 0, "new-file line")
	lines := fs.String("lines", "", "new-file lines N-M the note is about (instead of --line)")
	kind := fs.String("kind", "note", "note|spec")
	step := fs.String("step", "", "step id (default: current)")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	s, r, err := loadReview(ctx, e.dir)
	if err != nil {
		return err
	}
	text := strings.Join(fs.Args(), " ")
	if args[0] == "detail" {
		if text == "-" {
			data, err := readInput(e, "-")
			if err != nil {
				return err
			}
			text = string(data)
		}
		d := state.Detail{File: *file, Line: *line, Text: strings.TrimSpace(text)}
		if err := plan.SetDetail(r, *step, d); err != nil {
			return err
		}
		if err := s.store.Save(r); err != nil {
			return err
		}
		e.printf("detail %s %s:%d\n", cmp.Or(*step, r.Current), d.File, d.Line)
		return nil
	}
	a := state.Annotation{File: *file, Line: *line, Kind: *kind, Text: text}
	if *lines != "" {
		if a.Line, a.To, err = state.ParseLines(*lines); err != nil {
			return err
		}
	}
	if err := plan.AddNote(r, *step, a); err != nil {
		return err
	}
	if err := s.store.Save(r); err != nil {
		return err
	}
	e.printf("note %s %s:%d\n", cmp.Or(*step, r.Current), a.File, a.Line)
	return nil
}

func cmdDone(ctx context.Context, e env, _ []string) error {
	s, r, err := loadReview(ctx, e.dir)
	if err != nil {
		return err
	}
	if r.Worktree != "" {
		if err := s.repo.WorktreeRemove(ctx, r.Worktree); err != nil {
			return err
		}
		r.Worktree = ""
		if err := s.store.Save(r); err != nil {
			return err
		}
	}
	if err := s.store.ClearCurrent(); err != nil {
		return err
	}
	e.printf("review %s closed; state kept for a re-review\n", r.ID)
	return nil
}

func cmdList(ctx context.Context, e env, _ []string) error {
	s, err := openSession(ctx, e.dir)
	if err != nil {
		return err
	}
	ids, err := s.store.List()
	if err != nil {
		return err
	}
	current, _ := s.store.Current()
	for _, id := range ids {
		r, err := s.store.Load(id)
		if err != nil {
			return err
		}
		marker, title := " ", r.Source
		if id == current {
			marker = "*"
		}
		if r.MR != nil {
			title = r.MR.Title
		}
		cov := plan.CoverageOf(r)
		e.printf("%s %s  %d/%d steps  %s\n", marker, id, cov.Done+cov.Skipped, cov.Total, title)
		if path := s.exported(id); path != "" {
			e.printf("    export: %s\n", path)
		}
	}
	return nil
}

func announce(r *state.Review, st *state.Step) {
	if st == nil || st.Message == "" || st.Announced {
		return
	}
	st.Announced = true
	post := func(text string) {
		if !slices.ContainsFunc(r.Messages, func(m state.Message) bool {
			return m.Step == st.ID && m.Text == text
		}) {
			r.Messages = append(r.Messages, state.Message{Time: time.Now(), Step: st.ID, Text: text})
		}
	}
	if st.Intro != "" {
		post(cmp.Or(st.Chapter, "chapter") + "\n" + st.Intro)
	}
	post(st.Message)
	r.Messages = r.Messages[max(len(r.Messages)-state.MaxMessages, 0):]
}
