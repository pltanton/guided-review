package plan

import (
	"fmt"
	"path"
	"strings"

	"github.com/pltanton/guided-review/internal/state"
)

const (
	VerdictApprove = "approve"
	VerdictChanges = "changes"
	VerdictBlocked = "blocked"
)

func SuggestVerdict(r *state.Review) (verdict, why string) {
	round := max(r.Round, 1)
	var blockers, majors []string
	for _, c := range r.Comments {
		if c.Resolved || max(c.Round, 1) != round {
			continue
		}
		at := fmt.Sprintf("%s:%s", path.Base(c.File), c.Lines)
		switch c.Severity {
		case state.SeverityBlocker:
			blockers = append(blockers, at)
		case state.SeverityMajor:
			majors = append(majors, at)
		}
	}
	open := 0
	for _, d := range r.MyThreads() {
		if t := r.ThreadState(d); !t.Decided() || t.Verdict != state.VerdictResolve {
			open++
		}
	}
	switch {
	case len(blockers) > 0:
		return VerdictBlocked, counted(len(blockers), "blocker") + ": " + strings.Join(blockers, ", ")
	case len(majors) > 0:
		return VerdictChanges, counted(len(majors), "major") + ": " + strings.Join(majors, ", ")
	case open > 0:
		return VerdictChanges, counted(open, "thread") + " of yours still open"
	}
	return VerdictApprove, "no blocker or major, no open threads"
}

func Decisions(r *state.Review) string {
	var lines, pending, unchecked []string
	checked, risks := 0, 0
	for _, s := range r.Steps {
		switch s.Status {
		case state.StatusSkipped:
			lines = append(lines, fmt.Sprintf("- Skipped %s «%s»: %s", s.ID, s.Title, s.SkipReason))
		case state.StatusPending, state.StatusStale:
			pending = append(pending, s.ID)
		}
		for _, h := range s.Hotspots {
			risks++
			if h.Checked {
				checked++
			} else {
				unchecked = append(unchecked, fmt.Sprintf("«%s» (%s)", h.Q, s.ID))
			}
		}
	}
	if risks > 0 {
		line := fmt.Sprintf("- Risks checked: %d of %d", checked, risks)
		if len(unchecked) > 0 {
			line += "; not checked: " + strings.Join(unchecked, ", ")
		}
		lines = append(lines, line)
	}
	if len(pending) > 0 {
		lines = append(lines, "- Not reviewed yet: "+strings.Join(pending, ", "))
	}
	return strings.Join(lines, "\n")
}

func counted(n int, what string) string {
	if n == 1 {
		return "1 " + what
	}
	return fmt.Sprintf("%d %ss", n, what)
}
