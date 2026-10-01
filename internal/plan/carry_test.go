package plan_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/pltanton/guided-review/internal/diff"
	"github.com/pltanton/guided-review/internal/plan"
	"github.com/pltanton/guided-review/internal/state"
)

func TestCarry(t *testing.T) {
	steps := []state.Step{
		{ID: "s1", Status: state.StatusDone, Hunks: []state.StepHunk{{File: "a.go"}}},
		{
			ID: "s2", Status: state.StatusPending, DependsOn: []string{"s1"},
			Hunks:       []state.StepHunk{{File: "a.go", Lines: "3-9"}},
			Annotations: []state.Annotation{{File: "a.go", Line: 4, Kind: "note", Text: "x"}},
		},
		{
			ID: "s3", Status: state.StatusStale, Announced: true, DependsOn: []string{"s2"},
			Hunks: []state.StepHunk{
				{File: "b.go", Lines: "10-20"}, {File: "b.go", Lines: "30-40"}, {File: "gone.go"},
			},
			Hotspots: []state.Hotspot{
				{Cat: "money", Q: "?", File: "b.go", Line: 12, Detail: "z"},
			},
			Annotations: []state.Annotation{{File: "b.go", Line: 12, Kind: "note", Text: "y"}},
		},
		{ID: "r1-s9", FromRound: 1, Status: state.StatusPending,
			Hunks: []state.StepHunk{{File: "old.go", Lines: "1-2"}}},
		{ID: "s4", Status: state.StatusPending, Hunks: []state.StepHunk{{File: "gone.go"}}},
	}
	since := []diff.File{
		{Path: "b.go"}, {Path: "gone.go", Status: diff.Deleted}, {Path: "new.go", OldPath: "old.go"},
	}
	full := []diff.File{{Path: "a.go"}, {Path: "b.go"}, {Path: "new.go"}}

	got := plan.Carry(steps, 2, since, full)
	ids := make([]string, len(got))
	for i, st := range got {
		ids[i] = st.ID
		if st.Status != state.StatusPending || st.Announced {
			t.Fatalf("%s: carried steps start pending and unannounced: %+v", st.ID, st)
		}
	}
	if !slices.Equal(ids, []string{"r2-s2", "r2-s3", "r1-s9"}) {
		t.Fatalf("carried ids %v", ids)
	}
	s2, s3, s9 := got[0], got[1], got[2]
	if s2.FromRound != 2 || s2.Hunks[0].Lines != "3-9" || len(s2.Annotations) != 1 ||
		len(s2.DependsOn) != 0 {
		t.Fatalf("an unchanged file keeps its lines and notes: %+v", s2)
	}
	if !slices.Equal(s3.Hunks, []state.StepHunk{{File: "b.go"}}) || len(s3.Annotations) != 0 ||
		s3.Hotspots[0].Detail != "" || s3.Hotspots[0].Line != 0 || s3.Hotspots[0].Q != "?" ||
		!slices.Equal(s3.DependsOn, []string{"r2-s2"}) {
		t.Fatalf("a changed file becomes one whole-file hunk without line anchors: %+v", s3)
	}
	if s9.FromRound != 1 || s9.Hunks[0] != (state.StepHunk{File: "new.go"}) {
		t.Fatalf("a renamed file follows its new path, an old carry keeps its id: %+v", s9)
	}
}

func TestValidateWithCarried(t *testing.T) {
	r, files := fixture()
	r.RoundFiles = []string{"wire.go"}
	r.Carried = []state.Step{{ID: "r1-s1", Title: "rest", FromRound: 1,
		Hunks: []state.StepHunk{{File: "api/a.go"}}}}
	if errs := plan.Validate(plan.Plan{}, r, files); len(errs) == 0 ||
		!strings.Contains(errs[0].Error(), "not covered: wire.go") {
		t.Fatalf("round files still need a step, got %v", errs)
	}
	p := plan.Plan{Steps: []plan.Step{{ID: "r1-s1", Title: "x", Kind: "logic",
		Hunks: []state.StepHunk{{File: "wire.go"}}}}}
	if errs := plan.Validate(p, r, files); len(errs) != 1 ||
		!strings.Contains(errs[0].Error(), "carried") {
		t.Fatalf("a carried id is taken, got %v", errs)
	}
	p.Steps[0].ID, p.Steps[0].DependsOn = "n1", []string{"r1-s1"}
	if errs := plan.Validate(p, r, files); len(errs) != 0 {
		t.Fatalf("files outside the round are the carried steps' business: %v", errs)
	}
	plan.Apply(r, p)
	if len(r.Steps) != 2 || r.Steps[0].ID != "n1" || r.Steps[1].ID != "r1-s1" ||
		r.Steps[1].Status != state.StatusPending || r.Current != "n1" {
		t.Fatalf("carried steps follow the plan: %+v", r.Steps)
	}

	r.RoundFiles = nil
	if errs := plan.Validate(plan.Plan{}, r, files); len(errs) != 0 {
		t.Fatalf("an empty plan is fine when only carried steps are left: %v", errs)
	}
	plan.Apply(r, plan.Plan{})
	if len(r.Steps) != 1 || r.Current != "r1-s1" {
		t.Fatalf("an empty plan leaves the carried steps: %+v", r.Steps)
	}
}
