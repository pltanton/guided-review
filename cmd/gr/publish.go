package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
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

type pendingDraft struct {
	comment *state.Comment
	where   string
	draft   gitlab.DraftNote
}

func cmdPublish(ctx context.Context, e env, args []string) error {
	fs := flag.NewFlagSet("publish", flag.ContinueOnError)
	fs.SetOutput(e.stdout)
	dryRun := fs.Bool("dry-run", false, "print what would be posted and stop")
	verdict := fs.String("verdict", "", "approve|changes|blocked")
	decisions := fs.String("decisions", "", "decisions taken during the review and why (markdown)")
	decisionsFile := fs.String("decisions-file", "", "read decisions from a file (- for stdin)")
	approve := fs.Bool("approve", false, "also approve the MR")
	prepare := fs.Bool("prepare", false, "store the verdict and decisions for the viewer's publish button and print the preview")
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
	if *verdict == "" && r.Publish != nil {
		*verdict, *approve = r.Publish.Verdict, *approve || r.Publish.Approve
		if *decisions == "" {
			*decisions = r.Publish.Decisions
		}
	}
	if _, ok := verdicts[*verdict]; !ok {
		return errors.New("--verdict must be approve, changes or blocked (or prepare one with --prepare)")
	}
	if r.MR == nil {
		return errors.New("not a merge request review: nothing to publish to")
	}
	if len(r.Steps) == 0 {
		return errors.New("no plan yet: nothing reviewed")
	}
	if pending := plan.Gate(r); len(pending) > 0 {
		return fmt.Errorf("gate not passed, pending: %s (review or skip them first)", strings.Join(pending, " "))
	}
	raw, err := s.repo.Diff(ctx, r.BaseSHA, r.HeadSHA)
	if err != nil {
		return err
	}
	mrFiles, err := diff.Parse(raw)
	if err != nil {
		return err
	}

	var drafts []pendingDraft
	for i := range r.Comments {
		c := &r.Comments[i]
		if c.Published {
			continue
		}
		d, where := commentDraft(r, *c, mrFiles)
		drafts = append(drafts, pendingDraft{comment: c, where: where, draft: d})
	}
	round := max(r.Round, 1)
	withSummary := r.SummaryRound != round
	summary := summaryMarkdown(r, *verdict, *decisions)

	preview := func() {
		for _, d := range drafts {
			fmt.Fprintf(e.stdout, "--- %s\n%s\n\n", d.where, d.draft.Note)
		}
		if withSummary {
			fmt.Fprintf(e.stdout, "--- summary\n%s\n", summary)
		}
	}
	if *prepare {
		r.Publish = &state.PublishPlan{Verdict: *verdict, Decisions: *decisions, Approve: *approve}
		if err := s.store.Save(r); err != nil {
			return err
		}
		what := fmt.Sprintf("%d comments", len(drafts))
		if withSummary {
			what += " and the summary"
		}
		fmt.Fprintf(e.stdout, "prepared: %s — press P in the viewer to preview and publish\n\n", what)
		preview()
		return nil
	}
	if *dryRun {
		preview()
		return nil
	}
	if len(drafts) == 0 && !withSummary {
		fmt.Fprintln(e.stdout, "nothing new to publish")
		return nil
	}

	ref := gitlab.MRRef{Host: r.MR.Host, Project: r.MR.Project, IID: r.MR.IID}
	for _, d := range drafts {
		if d.comment.DraftID != 0 {
			continue
		}
		id, err := gitlab.CreateDraft(ctx, e.glab, ref, d.draft)
		if err != nil {
			return err
		}
		d.comment.DraftID = id
		if err := s.store.Save(r); err != nil {
			return err
		}
	}
	if withSummary && r.SummaryDraft == 0 {
		id, err := gitlab.CreateDraft(ctx, e.glab, ref, gitlab.DraftNote{Note: summary})
		if err != nil {
			return err
		}
		r.SummaryDraft = id
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
	what := fmt.Sprintf("%d comments", len(drafts))
	if withSummary {
		what += " and the summary"
	}
	fmt.Fprintf(e.stdout, "published %s to !%d\n%s\n", what, r.MR.IID, r.MR.URL)
	return nil
}

func readInput(e env, path string) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(e.stdin)
	}
	return os.ReadFile(path)
}

func commentDraft(r *state.Review, c state.Comment, mrFiles []diff.File) (gitlab.DraftNote, string) {
	start, end, _ := state.ParseLines(c.Lines)
	body := fmt.Sprintf("**%s** %s", c.Severity, c.Body)
	if c.Suggestion != "" {
		body += fmt.Sprintf("\n\n```suggestion:-%d+0\n%s\n```", end-start, c.Suggestion)
	}
	where := fmt.Sprintf("%s:%d", c.File, end)
	i := slices.IndexFunc(mrFiles, func(f diff.File) bool { return f.Path == c.File })
	if c.SHA != r.HeadSHA || i < 0 {
		return gitlab.DraftNote{Note: fmt.Sprintf("`%s:%s` %s", c.File, c.Lines, body)}, where + " (general note)"
	}
	f := mrFiles[i]
	oldPath := f.OldPath
	if oldPath == "" {
		oldPath = f.Path
	}
	pos := &gitlab.Position{
		PositionType: "text", BaseSHA: r.BaseSHA, StartSHA: r.StartSHA, HeadSHA: r.HeadSHA,
		OldPath: oldPath, NewPath: f.Path, NewLine: end,
	}
	if old, added := f.OldLineFor(end); !added {
		pos.OldLine = old
	}
	return gitlab.DraftNote{Note: body, Position: pos}, where
}

func summaryMarkdown(r *state.Review, verdict, decisions string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Guided review: %s\n\n", verdicts[verdict])
	if r.Round > 1 {
		fmt.Fprintf(&b, "Round %d.\n\n", r.Round)
	}
	if r.Summary != "" {
		fmt.Fprintf(&b, "%s\n\n", r.Summary)
	}
	if strings.TrimSpace(decisions) != "" {
		fmt.Fprintf(&b, "**Decisions**\n\n%s\n\n", strings.TrimSpace(decisions))
	}
	b.WriteString("| | Step | Notes |\n|---|---|---|\n")
	for _, st := range r.Steps {
		note := ""
		switch st.Status {
		case state.StatusSkipped:
			note = "skipped: " + st.SkipReason
		case state.StatusStale:
			note = "not reviewed: depends on a blocked step"
		}
		fmt.Fprintf(&b, "| %s | %s %s | %s |\n", statusGlyph[st.Status], st.ID, st.Title, note)
	}
	counts := map[state.Severity]int{}
	var resolved []string
	for _, c := range r.Comments {
		counts[c.Severity]++
		if c.Resolved {
			resolved = append(resolved, fmt.Sprintf("#%d", c.ID))
		}
	}
	fmt.Fprintf(&b, "\n**Comments:** %d blocker · %d major · %d minor · %d nit (inline)\n",
		counts[state.SeverityBlocker], counts[state.SeverityMajor], counts[state.SeverityMinor], counts[state.SeverityNit])
	if len(resolved) > 0 {
		fmt.Fprintf(&b, "**Resolved since the last round:** %s\n", strings.Join(resolved, ", "))
	}
	cov := plan.CoverageOf(r)
	fmt.Fprintf(&b, "**Coverage:** %d/%d steps, hotspots %d/%d; boilerplate %d files and generated %d files not reviewed line by line\n",
		cov.Done+cov.Skipped, cov.Total, cov.HotspotsReviewed, cov.Hotspots, cov.Boilerplate, cov.Generated)
	b.WriteString("\n<sub>guided-review</sub>\n")
	return b.String()
}
