package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pltanton/guided-review/internal/diff"
	"github.com/pltanton/guided-review/internal/gitlab"
	"github.com/pltanton/guided-review/internal/plan"
	"github.com/pltanton/guided-review/internal/state"
)

var verdicts = map[string]string{
	"approve": "approve ✅",
	"changes": "changes requested",
	"blocked": "blocked ⛔",
}

func cmdDiscussions(ctx context.Context, e env, _ []string) error {
	s, r, err := loadReview(ctx, e.dir)
	if err != nil {
		return err
	}
	if r.MR == nil {
		return errors.New("not a merge request review")
	}
	if err := syncDiscussions(ctx, e.glab, r); err != nil {
		return err
	}
	if err := s.store.Save(r); err != nil {
		return err
	}
	printDiscussions(e, r, true)
	return nil
}

func cmdPrepare(ctx context.Context, e env, args []string) error {
	fs := e.flags("prepare")
	verdict := fs.String("verdict", "", "approve|changes|blocked")
	decisions := fs.String("decisions", "", "decisions taken during the review and why (markdown)")
	decisionsFile := fs.String("decisions-file", "", "read decisions from a file (- for stdin)")
	approve := fs.Bool("approve", false, "the agent also approves the MR when publishing")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *decisionsFile != "" {
		data, err := readInput(e, *decisionsFile)
		if err != nil {
			return err
		}
		*decisions = string(data)
	}
	s, r, err := loadReview(ctx, e.dir)
	if err != nil {
		return err
	}
	switch _, ok := verdicts[*verdict]; {
	case !ok:
		return errors.New("--verdict must be approve, changes or blocked")
	case r.MR == nil && r.Mode != modeSelf:
		return errors.New("not a merge request review: nothing to publish to")
	case len(r.Steps) == 0:
		return errors.New("no plan yet: nothing reviewed")
	}
	if pending := plan.Gate(r); len(pending) > 0 {
		return fmt.Errorf("gate not passed, pending: %s (review or skip them first)",
			strings.Join(pending, " "))
	}
	r.Publish = &state.PublishPlan{Verdict: *verdict, Decisions: *decisions, Approve: *approve}
	if err := s.store.Save(r); err != nil {
		return err
	}
	e.println("prepared: the human reviews it and presses P in the viewer to finish")
	return nil
}

type export struct {
	Host     string   `json:"host"`
	API      string   `json:"api"`
	URL      string   `json:"url"`
	Verdict  string   `json:"verdict"`
	Approve  bool     `json:"approve"`
	Comments []int    `json:"comments"`
	Summary  bool     `json:"summary"`
	Drafts   []string `json:"drafts"`
}

func cmdExport(ctx context.Context, e env, args []string) error {
	fs := e.flags("export")
	dryRun := fs.Bool("dry-run", false, "print review.md and write nothing")
	if err := fs.Parse(args); err != nil {
		return err
	}
	s, r, err := loadReview(ctx, e.dir)
	if err != nil {
		return err
	}
	p := r.Publish
	if p == nil {
		return errors.New("nothing prepared: run gr prepare first")
	}
	if r.Mode == modeSelf {
		return exportSelf(e, r, *dryRun)
	}
	mrFiles, err := s.diff(ctx, r.BaseSHA, r.HeadSHA)
	if err != nil {
		return err
	}
	ref := gitlab.MRRef{Host: r.MR.Host, Project: r.MR.Project, IID: r.MR.IID}
	x := export{
		Host: ref.Host, API: ref.Path(""), URL: r.MR.URL, Verdict: p.Verdict, Approve: p.Approve,
	}
	var md strings.Builder
	var notes []gitlab.DraftNote
	for _, c := range r.Comments {
		if c.Published {
			continue
		}
		note, where := commentDraft(r, c, mrFiles)
		fmt.Fprintf(&md, "--- %s\n%s\n\n", where, note.Note)
		notes = append(notes, note)
		x.Comments = append(x.Comments, c.ID)
	}
	if x.Summary = r.SummaryRound != max(r.Round, 1); x.Summary {
		summary := summaryMarkdown(r, p.Verdict, p.Decisions)
		fmt.Fprintf(&md, "--- summary\n%s\n", summary)
		notes = append(notes, gitlab.DraftNote{Note: summary})
	}
	if *dryRun {
		e.printf("%s", md.String())
		return nil
	}
	dir := filepath.Join(e.exportDir, r.ID)
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, "drafts"), 0o755); err != nil {
		return err
	}
	for i, n := range notes {
		name := fmt.Sprintf("drafts/%02d.json", i+1)
		if err := writeJSON(filepath.Join(dir, name), n); err != nil {
			return err
		}
		x.Drafts = append(x.Drafts, name)
	}
	if err := writeJSON(filepath.Join(dir, "review.json"), x); err != nil {
		return err
	}
	mdPath := filepath.Join(dir, "review.md")
	if err := os.WriteFile(mdPath, []byte(md.String()), 0o644); err != nil {
		return err
	}
	e.println(dir)
	return nil
}

func cmdMarkPublished(ctx context.Context, e env, _ []string) error {
	s, r, err := loadReview(ctx, e.dir)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(e.exportDir, r.ID, "review.json"))
	if err != nil {
		return fmt.Errorf("no export to mark: %w", err)
	}
	var x export
	if err := json.Unmarshal(data, &x); err != nil {
		return err
	}
	for i := range r.Comments {
		if slices.Contains(x.Comments, r.Comments[i].ID) {
			r.Comments[i].Published = true
		}
	}
	if x.Summary {
		r.SummaryRound = max(r.Round, 1)
	}
	r.Publish = nil
	if err := s.store.Save(r); err != nil {
		return err
	}
	e.printf("marked %d comments published\n", len(x.Comments))
	return nil
}

const modeSelf = "self"

type fix struct {
	ID         int    `json:"id"`
	Severity   string `json:"severity"`
	File       string `json:"file"`
	Lines      string `json:"lines"`
	Body       string `json:"body"`
	Suggestion string `json:"suggestion,omitempty"`
}

func exportSelf(e env, r *state.Review, dryRun bool) error {
	p := r.Publish
	x := struct {
		Verdict   string `json:"verdict"`
		Decisions string `json:"decisions"`
		Fixes     []fix  `json:"fixes"`
	}{Verdict: p.Verdict, Decisions: p.Decisions}
	var md strings.Builder
	for _, c := range r.Comments {
		if c.Resolved {
			continue
		}
		fmt.Fprintf(&md, "--- %s:%s\n**%s** %s\n", c.File, c.Lines, c.Severity, c.Body)
		if c.Suggestion != "" {
			fmt.Fprintf(&md, "```suggestion\n%s\n```\n", c.Suggestion)
		}
		md.WriteString("\n")
		x.Fixes = append(x.Fixes, fix{
			ID: c.ID, Severity: string(c.Severity), File: c.File, Lines: c.Lines, Body: c.Body,
			Suggestion: c.Suggestion,
		})
	}
	fmt.Fprintf(&md, "--- summary\n%s\n", summaryMarkdown(r, p.Verdict, p.Decisions))
	if dryRun {
		e.printf("%s", md.String())
		return nil
	}
	dir := filepath.Join(e.exportDir, r.ID)
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(dir, "fixes.json"), x); err != nil {
		return err
	}
	mdPath := filepath.Join(dir, "review.md")
	if err := os.WriteFile(mdPath, []byte(md.String()), 0o644); err != nil {
		return err
	}
	e.println(dir)
	return nil
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func commentDraft(
	r *state.Review,
	c state.Comment,
	mrFiles []diff.File,
) (gitlab.DraftNote, string) {
	start, end, _ := state.ParseLines(c.Lines)
	body := fmt.Sprintf("**%s** %s", c.Severity, c.Body)
	if c.Suggestion != "" {
		body += fmt.Sprintf("\n\n```suggestion:-%d+0\n%s\n```", end-start, c.Suggestion)
	}
	where := fmt.Sprintf("%s:%d", c.File, end)
	i := slices.IndexFunc(mrFiles, func(f diff.File) bool { return f.Path == c.File })
	if c.SHA != r.HeadSHA || i < 0 {
		note := fmt.Sprintf("`%s:%s` %s", c.File, c.Lines, body)
		return gitlab.DraftNote{Note: note}, where + " (general note)"
	}
	f := mrFiles[i]
	pos := &gitlab.Position{
		PositionType: "text",
		BaseSHA:      r.BaseSHA,
		StartSHA:     r.StartSHA,
		HeadSHA:      r.HeadSHA,
		OldPath:      f.Path,
		NewPath:      f.Path,
		NewLine:      end,
	}
	if f.OldPath != "" {
		pos.OldPath = f.OldPath
	}
	if old, added := f.OldLineFor(end); !added {
		pos.OldLine = old
	}
	return gitlab.DraftNote{Note: body, Position: pos}, where
}

func summaryMarkdown(r *state.Review, verdict, decisions string) string {
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }
	w("## Guided review: %s\n\n", verdicts[verdict])
	if r.Round > 1 {
		w("Round %d.\n\n", r.Round)
	}
	if r.Summary != "" {
		w("%s\n\n", r.Summary)
	}
	if d := strings.TrimSpace(decisions); d != "" {
		w("**Decisions**\n\n%s\n\n", d)
	}
	w("| | Step | Notes |\n|---|---|---|\n")
	for _, st := range r.Steps {
		note := ""
		switch st.Status {
		case state.StatusSkipped:
			note = "skipped: " + st.SkipReason
		case state.StatusStale:
			note = "not reviewed: depends on a blocked step"
		}
		w("| %s | %s %s | %s |\n", st.Status.Glyph(), st.ID, st.Title, note)
	}
	var resolved []string
	for _, c := range r.Comments {
		if c.Resolved {
			resolved = append(resolved, fmt.Sprintf("#%d", c.ID))
		}
	}
	w("\n**Comments:** %s (inline)\n", severityCounts(r, " · "))
	if len(resolved) > 0 {
		w("**Resolved since the last round:** %s\n", strings.Join(resolved, ", "))
	}
	cov := plan.CoverageOf(r)
	w("**Coverage:** %d/%d steps, hotspots %d/%d; "+
		"boilerplate %d files and generated %d files not reviewed line by line\n",
		cov.Done+cov.Skipped, cov.Total, cov.HotspotsReviewed, cov.Hotspots,
		cov.Boilerplate, cov.Generated)
	w("\n<sub>guided-review</sub>\n")
	return b.String()
}
