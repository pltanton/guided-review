package main

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/aplotnikov/guided-review/internal/diff"
	"github.com/aplotnikov/guided-review/internal/gitlab"
	"github.com/aplotnikov/guided-review/internal/plan"
	"github.com/aplotnikov/guided-review/internal/state"
)

var verdicts = map[string]string{
	"approve": "approve ✅",
	"changes": "changes requested",
	"blocked": "blocked ⛔",
}

type draft struct {
	comment *state.Comment
	where   string
	note    gitlab.DraftNote
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

func cmdPublish(ctx context.Context, e env, args []string) error {
	fs := e.flags("publish")
	dryRun := fs.Bool("dry-run", false, "print what would be posted and stop")
	prepare := fs.Bool(
		"prepare",
		false,
		"store verdict and decisions for the viewer's P button, print the preview",
	)
	verdict := fs.String("verdict", "", "approve|changes|blocked")
	decisions := fs.String("decisions", "", "decisions taken during the review and why (markdown)")
	decisionsFile := fs.String("decisions-file", "", "read decisions from a file (- for stdin)")
	approve := fs.Bool("approve", false, "also approve the MR")
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
	if p := r.Publish; *verdict == "" && p != nil {
		*verdict, *approve = p.Verdict, *approve || p.Approve
		if *decisions == "" {
			*decisions = p.Decisions
		}
	}
	switch _, ok := verdicts[*verdict]; {
	case !ok:
		return errors.New(
			"--verdict must be approve, changes or blocked (or prepare one with --prepare)",
		)
	case r.MR == nil:
		return errors.New("not a merge request review: nothing to publish to")
	case len(r.Steps) == 0:
		return errors.New("no plan yet: nothing reviewed")
	}
	if pending := plan.Gate(r); len(pending) > 0 {
		return fmt.Errorf(
			"gate not passed, pending: %s (review or skip them first)",
			strings.Join(pending, " "),
		)
	}
	mrFiles, err := s.diff(ctx, r.BaseSHA, r.HeadSHA)
	if err != nil {
		return err
	}
	var drafts []draft
	for i := range r.Comments {
		if c := &r.Comments[i]; !c.Published {
			note, where := commentDraft(r, *c, mrFiles)
			drafts = append(drafts, draft{comment: c, where: where, note: note})
		}
	}
	round := max(r.Round, 1)
	withSummary := r.SummaryRound != round
	summary := summaryMarkdown(r, *verdict, *decisions)
	what := fmt.Sprintf("%d comments", len(drafts))
	if withSummary {
		what += " and the summary"
	}
	preview := func() {
		for _, d := range drafts {
			e.printf("--- %s\n%s\n\n", d.where, d.note.Note)
		}
		if withSummary {
			e.printf("--- summary\n%s\n", summary)
		}
	}

	switch {
	case *prepare:
		r.Publish = &state.PublishPlan{Verdict: *verdict, Decisions: *decisions, Approve: *approve}
		if err := s.store.Save(r); err != nil {
			return err
		}
		e.printf("prepared: %s — press P in the viewer to preview and publish\n\n", what)
		preview()
		return nil
	case *dryRun:
		preview()
		return nil
	case len(drafts) == 0 && !withSummary:
		e.println("nothing new to publish")
		return nil
	}

	ref := gitlab.MRRef{Host: r.MR.Host, Project: r.MR.Project, IID: r.MR.IID}
	for _, d := range drafts {
		if d.comment.DraftID != 0 {
			continue
		}
		if d.comment.DraftID, err = gitlab.CreateDraft(ctx, e.glab, ref, d.note); err != nil {
			return err
		}
		if err := s.store.Save(r); err != nil {
			return err
		}
	}
	if withSummary && r.SummaryDraft == 0 {
		note := gitlab.DraftNote{Note: summary}
		if r.SummaryDraft, err = gitlab.CreateDraft(ctx, e.glab, ref, note); err != nil {
			return err
		}
		if err := s.store.Save(r); err != nil {
			return err
		}
	}
	if err := gitlab.PublishDrafts(ctx, e.glab, ref); err != nil {
		return err
	}
	for _, d := range drafts {
		d.comment.Published = true
	}
	if withSummary {
		r.SummaryRound, r.SummaryDraft = round, 0
	}
	r.Publish = nil
	if err := s.store.Save(r); err != nil {
		return err
	}
	if *approve {
		if err := gitlab.Approve(ctx, e.glab, ref); err != nil {
			return err
		}
	}
	e.printf("published %s to !%d\n%s\n", what, r.MR.IID, r.MR.URL)
	return nil
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
