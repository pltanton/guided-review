package plan_test

import (
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
	}}
	files := []diff.File{
		{Path: "api/a.go", Hunks: []diff.Hunk{{NewStart: 10, NewLines: 5}}},
		{Path: "wire.go", Hunks: []diff.Hunk{{NewStart: 1, NewLines: 3}}},
		{Path: "a.pb.go", Hunks: []diff.Hunk{{NewStart: 1, NewLines: 100}}},
	}
	return r, files
}

func validPlan() plan.Plan {
	return plan.Plan{
		Briefing:    "task → solution",
		Boilerplate: []string{"wire.go"},
		Files:       []plan.File{{Path: "api/a.go", Order: 1, Why: "the guard"}},
		Hotspots:    []state.Hotspot{{Cat: "money", Q: "rounding?", File: "api/a.go", Line: 12}},
		Annotations: []state.Annotation{{File: "api/a.go", Line: 11, Kind: "note", Text: "reserves"}},
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*plan.Plan)
		want   string
	}{
		{"valid", func(*plan.Plan) {}, ""},
		{"empty", func(p *plan.Plan) { *p = plan.Plan{} }, ""},
		{"file not in diff", func(p *plan.Plan) { p.Files[0].Path = "nope.go" }, "file nope.go: not in diff"},
		{"file twice", func(p *plan.Plan) { p.Files = append(p.Files, p.Files[0]) }, "file api/a.go: listed twice"},
		{"bad order", func(p *plan.Plan) { p.Files[0].Order = -1 }, "order must be >= 0"},
		{"hotspot without question", func(p *plan.Plan) { p.Hotspots[0].Q = " " }, "hotspot api/a.go:12 has no question"},
		{"hotspot bad category", func(p *plan.Plan) { p.Hotspots[0].Cat = "perf" }, `category "perf"`},
		{"hotspot file", func(p *plan.Plan) { p.Hotspots[0].File = "" }, "hotspot :12: not in diff"},
		{"boilerplate not in diff", func(p *plan.Plan) { p.Boilerplate = []string{"x.go"} }, "boilerplate x.go: not in diff"},
		{"annotation file", func(p *plan.Plan) { p.Annotations[0].File = "x.go" }, "annotation x.go:11: not in diff"},
		{"annotation kind", func(p *plan.Plan) { p.Annotations[0].Kind = "todo" }, `kind "todo"`},
		{"annotation line", func(p *plan.Plan) { p.Annotations[0].Line = 0 }, "line must be >= 1"},
		{"annotation text", func(p *plan.Plan) { p.Annotations[0].Text = "" }, "empty text"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, files := fixture()
			p := validPlan()
			tt.mutate(&p)
			var msgs []string
			for _, e := range plan.Validate(p, files) {
				msgs = append(msgs, e.Error())
			}
			joined := strings.Join(msgs, "\n")
			if tt.want == "" && joined != "" {
				t.Fatalf("unexpected errors:\n%s", joined)
			}
			if tt.want != "" && !strings.Contains(joined, tt.want) {
				t.Fatalf("want error containing %q, got:\n%s", tt.want, joined)
			}
		})
	}
}

func TestParseRejectsUnknownFields(t *testing.T) {
	for _, in := range []string{"files:\n  - path: a.go\n    titel: typo\n", "steps: []\n"} {
		if _, err := plan.Parse([]byte(in)); err == nil {
			t.Errorf("%q: want error for unknown field", in)
		}
	}
}

func TestMergeReplacesAndAppends(t *testing.T) {
	r, _ := fixture()
	r.Comments = []state.Comment{{ID: 1}}
	plan.Merge(r, validPlan())
	r.Hotspots[0].Checked = true
	plan.Merge(r, plan.Plan{
		Files:       []plan.File{{Path: "api/a.go", Why: "the guard, now with a limit"}, {Path: "wire.go"}},
		Hotspots:    []state.Hotspot{{Cat: "money", Q: "rounding twice?", File: "api/a.go", Line: 12, Detail: "d"}, {Cat: "security", Q: "auth?", File: "wire.go", Line: 1}},
		Annotations: []state.Annotation{{File: "api/a.go", Line: 11, Kind: "note", Text: "reserves, longer"}},
	})
	a, w := r.File("api/a.go"), r.File("wire.go")
	if r.Briefing != "task → solution" || a.Order != 1 || a.Why != "the guard, now with a limit" || !a.Read || !w.Read || w.Why != "" {
		t.Fatalf("files after merge: %+v %+v briefing %q", a, w, r.Briefing)
	}
	if w.Tier != state.TierBoilerplate || r.File("a.pb.go").Tier != state.TierGenerated {
		t.Fatalf("tiers: %+v", r.Files)
	}
	if len(r.Hotspots) != 2 || r.Hotspots[0].Q != "rounding twice?" || !r.Hotspots[0].Checked || r.Hotspots[0].Detail != "d" {
		t.Fatalf("hotspots: %+v", r.Hotspots)
	}
	if len(r.Annotations) != 1 || r.Annotations[0].Text != "reserves, longer" {
		t.Fatalf("annotations: %+v", r.Annotations)
	}
	if len(r.Comments) != 1 {
		t.Fatal("Merge must keep comments")
	}
}

func TestCommentChecksHotspot(t *testing.T) {
	r, _ := fixture()
	plan.Merge(r, validPlan())
	c, err := plan.AddComment(r, state.Comment{File: "api/a.go", Lines: "11-13", Severity: state.SeverityMinor, Body: "b"})
	if err != nil || c.ID != 1 || c.Round != 1 {
		t.Fatalf("AddComment: %+v %v", c, err)
	}
	if h := r.Hotspots[0]; !h.Checked || h.Comment != 1 {
		t.Fatalf("a comment on the hotspot's line checks it off: %+v", h)
	}
	if err := plan.CheckHotspot(r, 1, false); err != nil || r.Hotspots[0].Checked {
		t.Fatalf("CheckHotspot undo: %v %+v", err, r.Hotspots[0])
	}
	if err := plan.CheckHotspot(r, 2, true); err == nil {
		t.Fatal("no hotspot #2")
	}
	for _, bad := range []state.Comment{
		{File: "api/a.go", Lines: "1", Severity: "huge", Body: "b"},
		{File: "nope.go", Lines: "1", Severity: state.SeverityNit, Body: "b"},
		{File: "api/a.go", Severity: state.SeverityNit, Body: "b"},
		{File: "api/a.go", Lines: "1", Severity: state.SeverityNit, Body: " "},
	} {
		if _, err := plan.AddComment(r, bad); err == nil {
			t.Errorf("AddComment(%+v) must fail", bad)
		}
	}
}
