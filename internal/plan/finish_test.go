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
	r := &state.Review{
		Files: []state.File{
			{Path: "a.go", Tier: state.TierCore, Seen: true},
			{Path: "x/b.go", Tier: state.TierCore},
			{Path: "gen.pb.go", Tier: state.TierGenerated},
		},
		Hotspots: []state.Hotspot{
			{Q: "rounding?", File: "a.go", Line: 3, Checked: true},
			{Q: "retry twice?", File: "x/b.go", Line: 9},
		},
	}
	want := "- Risks checked: 1 of 2; not checked: «retry twice?» (b.go:9)"
	if got := plan.Decisions(r); got != want {
		t.Errorf("Decisions =\n%s\nwant\n%s", got, want)
	}
	if got := plan.Unread(r); len(got) != 1 || got[0] != "x/b.go" {
		t.Errorf("Unread = %v", got)
	}
	if got := plan.Decisions(&state.Review{Hotspots: r.Hotspots}); got != want {
		t.Errorf("Decisions =\n%s\nwant\n%s", got, want)
	}
	if got := plan.Decisions(&state.Review{}); got != "" {
		t.Errorf("Decisions of an empty review = %q, want empty", got)
	}
}
