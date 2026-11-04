package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pltanton/guided-review/internal/plan"
	"github.com/pltanton/guided-review/internal/state"
)

func cmdStatus(ctx context.Context, e env, _ []string) error {
	s, r, err := loadReview(ctx, e.dir)
	if err != nil {
		return err
	}
	printHeader(e, s, r)
	if path := s.exported(r.ID); path != "" {
		e.printf("export: %s\n", path)
	}
	e.println()
	printFiles(e, r)
	e.printf("comments: %s\n", severityCounts(r, ", "))
	return nil
}

func printFiles(e env, r *state.Review) {
	flags := map[string]int{}
	checked := 0
	for _, h := range r.Hotspots {
		flags[h.File]++
		if h.Checked {
			checked++
		}
	}
	read, core := 0, 0
	for _, f := range r.FileOrder() {
		if f.Tier != state.TierCore {
			continue
		}
		core++
		order, mark := "  ", "·"
		if f.Order > 0 {
			order = fmt.Sprintf("%2d", f.Order)
		}
		if f.Read {
			read++
			mark = " "
		}
		if !f.Seen {
			mark += " ●"
		} else {
			mark += "  "
		}
		flag := "   "
		if n := flags[f.Path]; n > 0 {
			flag = fmt.Sprintf("⚑%-2d", n)
		}
		e.printf("%s %s %s %s", order, flag, mark, f.Path)
		if f.Why != "" {
			e.printf(" — %s", f.Why)
		}
		e.println()
	}
	tiers := map[state.Tier]int{}
	for _, f := range r.Files {
		tiers[f.Tier]++
	}
	e.printf("\nread by the agent %d/%d (· not yet), seen by you %d/%d (● not yet), hotspots %d/%d checked",
		read, core, core-len(plan.Unread(r)), core, checked, len(r.Hotspots))
	for _, t := range state.Tiers[1:] {
		if tiers[t] > 0 {
			e.printf(", %s %d files", t, tiers[t])
		}
	}
	e.println()
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
	fs := e.flags("plan " + args[0])
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
		p.Briefing = cmp.Or(p.Briefing, part.Briefing)
		p.Files = append(p.Files, part.Files...)
		p.Hotspots = append(p.Hotspots, part.Hotspots...)
		p.Annotations = append(p.Annotations, part.Annotations...)
	}
	_, r, err := updateReview(ctx, e.dir, func(s session, r *state.Review) error {
		files, err := s.reviewDiff(ctx, r)
		if err != nil {
			return err
		}
		if errs := plan.Validate(p, files); len(errs) > 0 {
			msgs := make([]string, len(errs))
			for i, err := range errs {
				msgs[i] = err.Error()
			}
			return fmt.Errorf("plan rejected:\n  - %s", strings.Join(msgs, "\n  - "))
		}
		plan.Merge(r, p)
		r.Progress = nil
		return nil
	})
	if err != nil {
		return err
	}
	e.printf("plan accepted: %d files, %d hotspots, %d notes in this part\n\n",
		len(p.Files), len(p.Hotspots), len(p.Annotations))
	printFiles(e, r)
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

func cmdComment(ctx context.Context, e env, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: gr comment add|list|edit|resolve|delete")
	}
	if args[0] == "list" {
		_, r, err := loadReview(ctx, e.dir)
		if err != nil {
			return err
		}
		for _, c := range r.Comments {
			status := ""
			if c.Resolved {
				status = "  [resolved]"
			}
			e.printf("#%d %s %s:%s  %s%s\n", c.ID, c.Severity, c.File, c.Lines, c.Body, status)
			if c.Suggestion != "" {
				e.printf("   suggestion:\n%s\n", indent(string(c.Suggestion), "     "))
			}
		}
		return nil
	}
	var msg string
	_, _, err := updateReview(ctx, e.dir, func(_ session, r *state.Review) error {
		switch args[0] {
		case "edit", "resolve", "delete":
			if len(args) < 2 {
				return fmt.Errorf("usage: gr comment %s ID", args[0])
			}
			id, err := parseID(args[1])
			if err != nil {
				return err
			}
			if args[0] == "delete" {
				if err := plan.DeleteComment(r, id); err != nil {
					return err
				}
				msg = fmt.Sprintf("comment #%d deleted", id)
				break
			}
			if args[0] == "resolve" {
				if err := plan.ResolveComment(r, id); err != nil {
					return err
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
			suggestion := fs.String("suggestion", "", "replacement text for the lines")
			if err := fs.Parse(args[1:]); err != nil {
				return err
			}
			c, err := plan.AddComment(r, state.Comment{
				File:       *file,
				Lines:      *lines,
				Severity:   state.Severity(*severity),
				Body:       strings.Join(fs.Args(), " "),
				Suggestion: state.Code(*suggestion),
			})
			if err != nil {
				return err
			}
			msg = fmt.Sprintf("comment #%d %s %s:%s", c.ID, c.Severity, c.File, c.Lines)
		default:
			return fmt.Errorf("unknown comment command %q", args[0])
		}
		return nil
	})
	if err != nil {
		return err
	}
	e.println(msg)
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
		err := s.store.Update(r.ID, func(r *state.Review) error {
			r.Worktree = ""
			return nil
		})
		if err != nil {
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
		core := 0
		for _, f := range r.Files {
			if f.Tier == state.TierCore {
				core++
			}
		}
		e.printf("%s %s  %d/%d files seen  %s\n", marker, id, core-len(plan.Unread(r)), core, title)
		if path := s.exported(id); path != "" {
			e.printf("    export: %s\n", path)
		}
	}
	return nil
}

func cmdCheck(ctx context.Context, e env, args []string) error {
	fs := e.flags("check")
	undo := fs.Bool("undo", false, "open the hotspot again")
	if err := fs.Parse(args); err != nil {
		return err
	}
	n, err := strconv.Atoi(fs.Arg(0))
	if fs.NArg() != 1 || err != nil {
		return errors.New("usage: gr check [--undo] N")
	}
	_, _, err = updateReview(ctx, e.dir, func(_ session, r *state.Review) error {
		return plan.CheckHotspot(r, n, !*undo)
	})
	return err
}
