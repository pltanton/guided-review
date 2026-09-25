package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"strings"

	"github.com/aplotnikov/guided-review/internal/plan"
	"github.com/aplotnikov/guided-review/internal/state"
)

var statusGlyph = map[state.StepStatus]string{
	state.StatusPending: "·",
	state.StatusDone:    "✓",
	state.StatusSkipped: "↷",
	state.StatusStale:   "~",
}

func cmdStatus(ctx context.Context, e env, args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(e.stdout)
	gate := fs.Bool("gate", false, "exit non-zero unless every step is reviewed")
	if err := fs.Parse(args); err != nil {
		return err
	}
	s, r, err := loadReview(ctx, e.dir)
	if err != nil {
		return err
	}
	w := e.stdout
	fmt.Fprintf(w, "review %s  %s..%s\n", r.ID, short(r.BaseSHA), short(r.HeadSHA))
	if r.MR != nil {
		fmt.Fprintf(w, "MR !%d %s\n", r.MR.IID, r.MR.Title)
	}
	if head, err := s.repo.Commit(ctx, "HEAD"); err == nil && head != r.HeadSHA {
		fmt.Fprintf(w, "warning: HEAD moved to %s, the review is pinned to %s\n", short(head), short(r.HeadSHA))
	}
	if len(r.Steps) == 0 {
		fmt.Fprintln(w, "no plan yet: pipe a plan to gr plan set")
		if *gate {
			return errors.New("gate not passed")
		}
		return nil
	}
	fmt.Fprintln(w)
	for _, st := range r.Steps {
		cursor := " "
		if st.ID == r.Current {
			cursor = ">"
		}
		hotspot := ""
		if len(st.Hotspots) > 0 {
			hotspot = " ⚑"
		}
		fmt.Fprintf(w, "%s %s %s  %s%s\n", cursor, statusGlyph[st.Status], st.ID, st.Title, hotspot)
	}
	cov := plan.CoverageOf(r)
	fmt.Fprintf(w, "\ncore %d/%d reviewed (%d skipped, %d stale), hotspots %d/%d, boilerplate %d files, generated %d files not reviewed\n",
		cov.Done+cov.Skipped, cov.Total, cov.Skipped, cov.Stale, cov.HotspotsReviewed, cov.Hotspots, cov.Boilerplate, cov.Generated)
	counts := map[state.Severity]int{}
	for _, c := range r.Comments {
		counts[c.Severity]++
	}
	fmt.Fprintf(w, "comments: %d blocker, %d major, %d minor, %d nit\n",
		counts[state.SeverityBlocker], counts[state.SeverityMajor], counts[state.SeverityMinor], counts[state.SeverityNit])
	if pending := plan.Gate(r); len(pending) > 0 {
		fmt.Fprintf(w, "gate: not passed, pending: %s\n", strings.Join(pending, " "))
		if *gate {
			return errors.New("gate not passed")
		}
		return nil
	}
	fmt.Fprintln(w, "gate: passed")
	return nil
}
