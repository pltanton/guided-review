package plan_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/pltanton/guided-review/internal/diff"
	"github.com/pltanton/guided-review/internal/plan"
	"github.com/pltanton/guided-review/internal/state"
)

func fixture() (*state.Review, []diff.File) {
	r := &state.Review{Files: []state.File{
		{Path: "api/a.go", Tier: state.TierCore},
		{Path: "wire.go", Tier: state.TierCore},
		{Path: "a.pb.go", Tier: state.TierGenerated},
		{Path: "gone.go", Tier: state.TierCore},
	}}
	files := []diff.File{
		{
			Path:  "api/a.go",
			Hunks: []diff.Hunk{{NewStart: 10, NewLines: 5}, {NewStart: 40, NewLines: 0}},
		},
		{Path: "wire.go", Hunks: []diff.Hunk{{NewStart: 1, NewLines: 3}}},
		{Path: "a.pb.go", Hunks: []diff.Hunk{{NewStart: 1, NewLines: 100}}},
		{Path: "gone.go", Status: diff.Deleted, Hunks: []diff.Hunk{{OldStart: 1, OldLines: 4}}},
	}
	return r, files
}

func validPlan() plan.Plan {
	return plan.Plan{
		Summary:     "task → solution",
		Boilerplate: []string{"wire.go"},
		Steps: []state.Step{
			{
				ID:    "s1",
				Title: "contract",
				Kind:  "contract",
				Hunks: []state.StepHunk{{File: "api/a.go", Lines: "1-20"}, {File: "gone.go"}},
			},
			{
				ID:        "s2",
				Title:     "logic",
				Kind:      "logic",
				Hunks:     []state.StepHunk{{File: "api/a.go", Lines: "38-45"}},
				DependsOn: []string{"s1"},
				Hotspots:  []state.Hotspot{{Cat: "money", Q: "rounding?"}},
			},
		},
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*plan.Plan)
		want   string
	}{
		{"valid", func(*plan.Plan) {}, ""},
		{"no steps", func(p *plan.Plan) { p.Steps = nil }, "plan has no steps"},
		{
			"uncovered hunk",
			func(p *plan.Plan) { p.Steps = p.Steps[:1] },
			"not covered: api/a.go:40(del)",
		},
		{"uncovered file", func(p *plan.Plan) { p.Boilerplate = nil }, "not covered: wire.go:1-3"},
		{
			"deleted file needs whole-file step",
			func(p *plan.Plan) { p.Steps[0].Hunks[1].Lines = "1-4" },
			"not covered: gone.go:0(del)",
		},
		{
			"duplicate id",
			func(p *plan.Plan) { p.Steps[1].ID = "s1"; p.Steps[1].DependsOn = nil },
			"step s1: duplicate id",
		},
		{
			"unknown file",
			func(p *plan.Plan) { p.Steps[0].Hunks[0].File = "nope.go" },
			"step s1: nope.go not in diff",
		},
		{"bad lines", func(p *plan.Plan) { p.Steps[0].Hunks[0].Lines = "20-1" }, `lines "20-1"`},
		{
			"unknown dep",
			func(p *plan.Plan) { p.Steps[1].DependsOn = []string{"s9"} },
			"step s2: depends_on s9: no such step",
		},
		{
			"self dep",
			func(p *plan.Plan) { p.Steps[0].DependsOn = []string{"s1"} },
			"step s1: depends on itself",
		},
		{
			"cycle",
			func(p *plan.Plan) { p.Steps[0].DependsOn = []string{"s2"} },
			"depends_on cycle: s1 → s2 → s1",
		},
		{
			"hotspot without question",
			func(p *plan.Plan) { p.Steps[1].Hotspots[0].Q = " " },
			"step s2: hotspot money has no question",
		},
		{
			"hotspot bad category",
			func(p *plan.Plan) { p.Steps[1].Hotspots[0].Cat = "perf" },
			`step s2: hotspot category "perf"`,
		},
		{
			"boilerplate not in diff",
			func(p *plan.Plan) { p.Boilerplate = append(p.Boilerplate, "x.go") },
			"boilerplate x.go: not in diff",
		},
		{"annotation ok", func(p *plan.Plan) {
			p.Steps[0].Annotations = []state.Annotation{
				{File: "api/a.go", Line: 12, Kind: "note", Text: "reserves"},
			}
		}, ""},
		{"annotation file", func(p *plan.Plan) {
			p.Steps[0].Annotations = []state.Annotation{
				{File: "x.go", Line: 1, Kind: "note", Text: "t"},
			}
		}, "step s1: annotation x.go:1: not in diff"},
		{"annotation kind", func(p *plan.Plan) {
			p.Steps[0].Annotations = []state.Annotation{
				{File: "api/a.go", Line: 1, Kind: "todo", Text: "t"},
			}
		}, `step s1: annotation api/a.go:1: kind "todo"`},
		{"annotation line", func(p *plan.Plan) {
			p.Steps[0].Annotations = []state.Annotation{
				{File: "api/a.go", Kind: "note", Text: "t"},
			}
		}, "step s1: annotation api/a.go:0: line must be >= 1"},
		{"annotation text", func(p *plan.Plan) {
			p.Steps[0].Annotations = []state.Annotation{{File: "api/a.go", Line: 1, Kind: "spec"}}
		}, "step s1: annotation api/a.go:1: empty text"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, files := fixture()
			p := validPlan()
			tt.mutate(&p)
			errs := plan.Validate(p, r, files)
			var msgs []string
			for _, e := range errs {
				msgs = append(msgs, e.Error())
			}
			joined := strings.Join(msgs, "\n")
			if tt.want == "" && len(errs) > 0 {
				t.Fatalf("unexpected errors:\n%s", joined)
			}
			if tt.want != "" && !strings.Contains(joined, tt.want) {
				t.Fatalf("want error containing %q, got:\n%s", tt.want, joined)
			}
		})
	}
}

func TestParseRejectsUnknownFields(t *testing.T) {
	if _, err := plan.Parse([]byte("steps:\n  - id: s1\n    titel: typo\n")); err == nil {
		t.Fatal("want error for unknown field")
	}
}

func TestApply(t *testing.T) {
	r, _ := fixture()
	r.Comments = []state.Comment{{ID: 1}}
	p := validPlan()
	p.Steps[0].Status = state.StatusDone
	plan.Apply(r, p)
	if r.Current != "s1" || r.Summary != p.Summary || len(r.Steps) != 2 {
		t.Fatalf("Apply: %+v", r)
	}
	for _, s := range r.Steps {
		if s.Status != state.StatusPending {
			t.Fatalf("step %s status %q, want pending", s.ID, s.Status)
		}
	}
	if r.File("wire.go").Tier != state.TierBoilerplate ||
		r.File("a.pb.go").Tier != state.TierGenerated ||
		r.File("api/a.go").Tier != state.TierCore {
		t.Fatalf("tiers: %+v", r.Files)
	}
	if len(r.Comments) != 1 {
		t.Fatal("Apply must keep comments")
	}
}

func TestStepSizeLimit(t *testing.T) {
	big := diff.Hunk{NewStart: 1, NewLines: 400}
	for range 400 {
		big.Lines = append(big.Lines, diff.Line{Kind: '+', Text: "x"})
	}
	r := &state.Review{Files: []state.File{{Path: "big.go", Tier: state.TierCore}}}
	files := []diff.File{{Path: "big.go", Hunks: []diff.Hunk{big}}}
	step := func(lines, why string) state.Step {
		return state.Step{ID: "s1", Title: "t", Hunks: []state.StepHunk{{File: "big.go", Lines: lines}},
			WhyBig: why}
	}
	tests := []struct {
		name    string
		steps   []state.Step
		wantErr bool
	}{
		{"whole file", []state.Step{step("", "")}, true},
		{"why_big given", []state.Step{step("", "one generated-like table")}, false},
		{"split by range", []state.Step{step("1-250", ""), {ID: "s2", Title: "t",
			Hunks: []state.StepHunk{{File: "big.go", Lines: "251-400"}}}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := plan.Validate(plan.Plan{Steps: tt.steps}, r, files)
			got := slices.ContainsFunc(errs, func(e error) bool {
				return strings.Contains(e.Error(), "over 300")
			})
			if got != tt.wantErr {
				t.Fatalf("size error = %v, want %v (%v)", got, tt.wantErr, errs)
			}
		})
	}
}

func TestChaptersStayTogether(t *testing.T) {
	r, files := fixture()
	p, err := plan.Parse([]byte(`steps:
  - {id: s1, title: a, kind: logic, chapter: A, hunks: [{file: api/a.go}]}
  - {id: s2, title: b, kind: logic, chapter: B, hunks: [{file: wire.go}]}
  - {id: s3, title: c, kind: logic, chapter: A, hunks: [{file: gone.go}]}
`))
	if err != nil {
		t.Fatal(err)
	}
	errs := plan.Validate(p, r, files)
	if !slices.ContainsFunc(errs, func(e error) bool { return strings.Contains(e.Error(), "is split") }) {
		t.Fatalf("a split chapter must be rejected: %v", errs)
	}
}

func TestDetailsFromPlan(t *testing.T) {
	r, _ := fixture()
	p, err := plan.Parse([]byte(`steps:
  - id: s1
    title: a
    kind: logic
    hunks: [{file: api/a.go}]
    hotspots: [{cat: money, q: "rounding?", line: 7, detail: "float64 before the DB write"}]
    annotations:
      - {file: api/a.go, line: 3, to: 5, kind: note, text: t, detail: "reserve opens the tx"}
`))
	if err != nil {
		t.Fatal(err)
	}
	plan.Apply(r, p)
	st := r.Step("s1")
	if d, ok := st.Detail("api/a.go", 5); !ok || d != "reserve opens the tx" {
		t.Fatalf("annotation detail sits on the note's last line, got %q %v", d, ok)
	}
	if d, ok := st.Detail("api/a.go", 7); !ok || d != "float64 before the DB write" {
		t.Fatalf("hotspot detail on its line, got %q %v", d, ok)
	}
}

func TestHotspotNeedsFileInMultiFileStep(t *testing.T) {
	r, files := fixture()
	p, err := plan.Parse([]byte(`steps:
  - id: s1
    title: a
    kind: logic
    hunks: [{file: api/a.go}, {file: wire.go}]
    hotspots: [{cat: money, q: "rounding?", line: 3}]
`))
	if err != nil {
		t.Fatal(err)
	}
	errs := plan.Validate(p, r, files)
	if !slices.ContainsFunc(errs, func(e error) bool { return strings.Contains(e.Error(), "needs file") }) {
		t.Fatalf("an ambiguous hotspot must be rejected: %v", errs)
	}
}
