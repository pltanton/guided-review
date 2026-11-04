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

var Verdicts = []string{VerdictApprove, VerdictChanges, VerdictBlocked}

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
	open := len(r.OpenThreads())
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

func Unread(r *state.Review) []string {
	var out []string
	for _, f := range r.Files {
		if f.Tier == state.TierCore && !f.Seen && r.InRound(f.Path) {
			out = append(out, f.Path)
		}
	}
	return out
}

func Decisions(r *state.Review) string {
	var lines, unchecked []string
	checked := 0
	for _, h := range r.Hotspots {
		if h.Checked {
			checked++
		} else {
			unchecked = append(unchecked, fmt.Sprintf("«%s» (%s:%d)", h.Q, path.Base(h.File), h.Line))
		}
	}
	if len(r.Hotspots) > 0 {
		line := fmt.Sprintf("- Risks checked: %d of %d", checked, len(r.Hotspots))
		if len(unchecked) > 0 {
			line += "; not checked: " + strings.Join(unchecked, ", ")
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func counted(n int, what string) string {
	if n == 1 {
		return "1 " + what
	}
	return fmt.Sprintf("%d %ss", n, what)
}
