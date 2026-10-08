package plan_test

import (
	"testing"

	"github.com/pltanton/guided-review/internal/plan"
	"github.com/pltanton/guided-review/internal/state"
)

func TestSuggestVerdict(t *testing.T) {
	tests := []struct {
		name        string
		give        []state.Comment
		wantVerdict string
		wantWhy     string
	}{
		{"clean", nil, plan.VerdictApprove, "no blocker or major, no open threads"},
		{"nits only", []state.Comment{{Severity: state.SeverityNit, File: "a.go", Lines: "1"}},
			plan.VerdictApprove, "no blocker or major, no open threads"},
		{"major", []state.Comment{{Severity: state.SeverityMajor, File: "x/a.go", Lines: "3-4"}},
			plan.VerdictChanges, "1 major: a.go:3-4"},
		{"blocker wins", []state.Comment{
			{Severity: state.SeverityMajor, File: "a.go", Lines: "1"},
			{Severity: state.SeverityBlocker, File: "b.go", Lines: "2"},
			{Severity: state.SeverityBlocker, File: "c.go", Lines: "3"},
		}, plan.VerdictBlocked, "2 blockers: b.go:2, c.go:3"},
		{"resolved blocker does not count", []state.Comment{
			{Severity: state.SeverityBlocker, File: "b.go", Lines: "2", Resolved: true},
		}, plan.VerdictApprove, "no blocker or major, no open threads"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &state.Review{Comments: tt.give}
			gotVerdict, gotWhy := plan.SuggestVerdict(r)
			if gotVerdict != tt.wantVerdict || gotWhy != tt.wantWhy {
				t.Errorf("SuggestVerdict = %q, %q, want %q, %q",
					gotVerdict, gotWhy, tt.wantVerdict, tt.wantWhy)
			}
		})
	}
}

func TestDecisions(t *testing.T) {
	r := &state.Review{Steps: []state.Step{
		{ID: "s1", Status: state.StatusDone, Hotspots: []state.Hotspot{{Q: "rounding?", Checked: true}}},
		{ID: "s2", Title: "wiring", Status: state.StatusSkipped, SkipReason: "trivial"},
		{ID: "s3", Status: state.StatusPending, Hotspots: []state.Hotspot{{Q: "retry twice?"}}},
	}}
	want := "- Skipped s2 «wiring»: trivial\n" +
		"- Risks checked: 1 of 2; not checked: «retry twice?» (s3)\n" +
		"- Not reviewed yet: s3"
	if got := plan.Decisions(r); got != want {
		t.Errorf("Decisions =\n%s\nwant\n%s", got, want)
	}
	if got := plan.Decisions(&state.Review{}); got != "" {
		t.Errorf("Decisions of an empty review = %q, want empty", got)
	}
}
