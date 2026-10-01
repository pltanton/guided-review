package plan_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/pltanton/guided-review/internal/plan"
	"github.com/pltanton/guided-review/internal/state"
)

func flowReview() *state.Review {
	step := func(id string, deps ...string) state.Step {
		return state.Step{ID: id, Title: id, Status: state.StatusPending, DependsOn: deps}
	}
	r := &state.Review{
		HeadSHA: "head",
		Files: []state.File{
			{Path: "a.go", Tier: state.TierCore},
			{Path: "w.go", Tier: state.TierBoilerplate},
			{Path: "x.pb.go", Tier: state.TierGenerated},
		},
		Steps: []state.Step{
			step("s1"),
			step("s2", "s1"),
			step("s3", "s2"),
			step("s4"),
			step("s5", "s1"),
		},
	}
	r.Steps[4].Hotspots = []state.Hotspot{{Cat: "money", Q: "?"}}
	r.Current = "s1"
	return r
}

func TestNextAndSkip(t *testing.T) {
	r := flowReview()
	st, err := plan.Next(r)
	if err != nil || st.ID != "s2" || r.Step("s1").Status != state.StatusDone {
		t.Fatalf("Next: %v, %v, %+v", st, err, r.Steps)
	}
	if _, err := plan.Skip(r, " "); err == nil {
		t.Fatal("Skip without reason must fail")
	}
	st, err = plan.Skip(r, "covered by s1")
	if err != nil || st.ID != "s3" || r.Step("s2").Status != state.StatusSkipped ||
		r.Step("s2").SkipReason != "covered by s1" {
		t.Fatalf("Skip: %v, %v", st, err)
	}
	if err := plan.Goto(r, "s1"); err != nil || r.Current != "s1" {
		t.Fatalf("Goto: %v", err)
	}
	st, err = plan.Next(r)
	if err != nil || st.ID != "s3" {
		t.Fatalf("Next after goto wraps to first pending: %v, %v", st, err)
	}
	for range 3 {
		st, err = plan.Next(r)
	}
	if !errors.Is(err, plan.ErrDone) || st != nil {
		t.Fatalf("want ErrDone, got %v, %v", st, err)
	}
	if err := plan.Goto(r, "nope"); err == nil {
		t.Fatal("Goto unknown step must fail")
	}
}

func TestAddCommentBlocker(t *testing.T) {
	r := flowReview()
	c, imp, err := plan.AddComment(
		r,
		state.Comment{
			File:     "a.go",
			Lines:    "3-4",
			Severity: state.SeverityBlocker,
			Body:     "wrong approach",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if c.ID != 1 || c.Step != "s1" || c.SHA != "head" {
		t.Fatalf("defaults not filled: %+v", c)
	}
	if !reflect.DeepEqual(imp.Stale, []string{"s2", "s3"}) ||
		!reflect.DeepEqual(imp.MayChange, []string{"s5"}) {
		t.Fatalf("impact: %+v", imp)
	}
	if r.Step("s3").Status != state.StatusStale || r.Step("s5").Status != state.StatusPending ||
		!r.Step("s5").MayChange {
		t.Fatalf("steps: %+v", r.Steps)
	}
	c2, _, err := plan.AddComment(
		r,
		state.Comment{File: "a.go", Lines: "9", Severity: state.SeverityNit, Body: "rename"},
	)
	if err != nil || c2.ID != 2 {
		t.Fatalf("second comment: %+v, %v", c2, err)
	}
}

func TestAddCommentMajor(t *testing.T) {
	r := flowReview()
	_, imp, err := plan.AddComment(
		r,
		state.Comment{File: "a.go", Lines: "1", Severity: state.SeverityMajor, Body: "rework"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(imp.Stale) != 0 || !reflect.DeepEqual(imp.MayChange, []string{"s2", "s3", "s5"}) {
		t.Fatalf("impact: %+v", imp)
	}
}

func TestAddCommentValidation(t *testing.T) {
	bad := []state.Comment{
		{File: "a.go", Lines: "1", Severity: "huge", Body: "x"},
		{File: "a.go", Lines: "1", Severity: state.SeverityNit, Body: " "},
		{File: "nope.go", Lines: "1", Severity: state.SeverityNit, Body: "x"},
		{File: "a.go", Lines: "", Severity: state.SeverityNit, Body: "x"},
		{File: "a.go", Lines: "1", Severity: state.SeverityNit, Body: "x", Step: "s9"},
	}
	for _, c := range bad {
		if _, _, err := plan.AddComment(flowReview(), c); err == nil {
			t.Errorf("AddComment(%+v): want error", c)
		}
	}
}

func TestGateAndCoverage(t *testing.T) {
	r := flowReview()
	if got := plan.Gate(r); !reflect.DeepEqual(got, []string{"s1", "s2", "s3", "s4", "s5"}) {
		t.Fatalf("Gate = %v", got)
	}
	r.Steps[0].Status = state.StatusDone
	r.Steps[1].Status = state.StatusStale
	r.Steps[2].Status = state.StatusSkipped
	r.Steps[3].Status = state.StatusDone
	r.Steps[4].Status = state.StatusDone
	if got := plan.Gate(r); len(got) != 0 {
		t.Fatalf("Gate = %v, want pass", got)
	}
	want := plan.Coverage{
		Total:            5,
		Done:             3,
		Skipped:          1,
		Stale:            1,
		Hotspots:         1,
		HotspotsReviewed: 1,
		Boilerplate:      1,
		Generated:        1,
	}
	if got := plan.CoverageOf(r); got != want {
		t.Fatalf("CoverageOf = %+v, want %+v", got, want)
	}
}

func TestAddNoteAndResolve(t *testing.T) {
	r := flowReview()
	note := state.Annotation{File: "a.go", Line: 3, Kind: "note", Text: "retry wrapper"}
	if err := plan.AddNote(r, "", note); err != nil {
		t.Fatal(err)
	}
	if got := r.Step("s1").Annotations; len(got) != 1 || got[0].Text != "retry wrapper" {
		t.Fatalf("annotations: %+v", got)
	}
	if err := plan.AddNote(r, "s9", note); err == nil {
		t.Fatal("unknown step must fail")
	}
	if err := plan.AddNote(r, "", state.Annotation{File: "a.go", Line: 3, Kind: "note"}); err == nil {
		t.Fatal("empty text must fail")
	}
	c, _, err := plan.AddComment(
		r,
		state.Comment{File: "a.go", Lines: "1", Severity: state.SeverityNit, Body: "x"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plan.ResolveComment(r, c.ID); err != nil || !r.Comments[0].Resolved {
		t.Fatalf("resolve: %v %+v", err, r.Comments)
	}
	if _, err := plan.ResolveComment(r, 99); err == nil {
		t.Fatal("unknown comment must fail")
	}
}

func TestEditComment(t *testing.T) {
	r := flowReview()
	c, _, _ := plan.AddComment(
		r,
		state.Comment{File: "a.go", Lines: "1", Severity: state.SeverityNit, Body: "x"},
	)
	if _, err := plan.EditComment(r, c.ID, "better text", state.SeverityMinor); err != nil {
		t.Fatal(err)
	}
	if got := r.Comments[0]; got.Body != "better text" || got.Severity != state.SeverityMinor {
		t.Fatalf("edited: %+v", got)
	}
	if _, err := plan.EditComment(r, c.ID, "keep severity", ""); err != nil ||
		r.Comments[0].Severity != state.SeverityMinor {
		t.Fatalf("empty severity must keep it: %v %+v", err, r.Comments[0])
	}
	edit := func(id int, body string, sev state.Severity) error {
		_, err := plan.EditComment(r, id, body, sev)
		return err
	}
	if edit(99, "x", "") == nil || edit(c.ID, " ", "") == nil || edit(c.ID, "x", "huge") == nil {
		t.Fatal("bad edits must fail")
	}
}

func TestDeleteComment(t *testing.T) {
	r := flowReview()
	c, _, err := plan.AddComment(r, state.Comment{
		File: "a.go", Lines: "3", Severity: state.SeverityBlocker, Body: "wrong",
	})
	if err != nil {
		t.Fatal(err)
	}
	imp, err := plan.DeleteComment(r, c.ID)
	if err != nil || !reflect.DeepEqual(imp.Restored, []string{"s2", "s3"}) || len(r.Comments) != 0 {
		t.Fatalf("DeleteComment = %+v, %v; comments %+v", imp, err, r.Comments)
	}
	if r.Step("s5").MayChange {
		t.Fatal("may-change must be cleared with its only cause")
	}
	if _, err := plan.DeleteComment(r, c.ID); err == nil {
		t.Fatal("deleting a missing comment must fail")
	}
	r.Comments = append(r.Comments, state.Comment{ID: 9, Published: true})
	if _, err := plan.DeleteComment(r, 9); err == nil {
		t.Fatal("a published comment must not be deleted locally")
	}
}

func blockedReview(t *testing.T) (*state.Review, state.Comment) {
	t.Helper()
	r := flowReview()
	c, _, err := plan.AddComment(r, state.Comment{
		File: "a.go", Lines: "3", Severity: state.SeverityBlocker, Body: "wrong",
	})
	if err != nil {
		t.Fatal(err)
	}
	return r, c
}

func TestResolveBlockerRestoresStale(t *testing.T) {
	r, c := blockedReview(t)
	imp, err := plan.ResolveComment(r, c.ID)
	if err != nil || !reflect.DeepEqual(imp.Restored, []string{"s2", "s3"}) {
		t.Fatalf("ResolveComment = %+v, %v", imp, err)
	}
	if r.Step("s2").Status != state.StatusPending || r.Step("s5").MayChange {
		t.Fatalf("steps: %+v", r.Steps)
	}
}

func TestEditCommentSeverityRecomputesStale(t *testing.T) {
	r, c := blockedReview(t)
	imp, err := plan.EditComment(r, c.ID, "local rework after all", state.SeverityMajor)
	if err != nil || !reflect.DeepEqual(imp.Restored, []string{"s2", "s3"}) ||
		!reflect.DeepEqual(imp.MayChange, []string{"s2", "s3"}) {
		t.Fatalf("downgrade to major = %+v, %v", imp, err)
	}
	if r.Step("s3").Status != state.StatusPending || !r.Step("s3").MayChange {
		t.Fatalf("steps: %+v", r.Steps)
	}
	imp, err = plan.EditComment(r, c.ID, "wrong approach", state.SeverityBlocker)
	if err != nil || !reflect.DeepEqual(imp.Stale, []string{"s2", "s3"}) {
		t.Fatalf("upgrade to blocker = %+v, %v", imp, err)
	}
	imp, err = plan.EditComment(r, c.ID, "fine", state.SeverityMinor)
	if err != nil || !reflect.DeepEqual(imp.Restored, []string{"s2", "s3"}) ||
		r.Step("s5").MayChange {
		t.Fatalf("downgrade to minor = %+v, %v; steps %+v", imp, err, r.Steps)
	}
}

func TestEarlierRoundBlockerDoesNotStaleNewSteps(t *testing.T) {
	r := flowReview()
	r.Round = 2
	r.Comments = []state.Comment{
		{ID: 1, Step: "s1", Severity: state.SeverityBlocker, Body: "old", Round: 1},
	}
	c, imp, err := plan.AddComment(r, state.Comment{
		File: "a.go", Lines: "1", Severity: state.SeverityNit, Body: "x",
	})
	if err != nil || len(imp.Stale) != 0 || r.Step("s2").Status != state.StatusPending {
		t.Fatalf("AddComment = %+v, %+v, %v; steps %+v", c, imp, err, r.Steps)
	}
	r.Steps[1].Status = state.StatusStale
	if imp, _ := plan.DeleteComment(r, c.ID); !reflect.DeepEqual(imp.Restored, []string{"s2"}) {
		t.Fatalf("DeleteComment = %+v", imp)
	}
}
