package state_test

import (
	"testing"
	"time"

	"github.com/pltanton/guided-review/internal/state"
)

func linkReview() *state.Review {
	return &state.Review{
		MR: &state.MR{Me: "me"},
		Comments: []state.Comment{
			{ID: 1, File: "a.go", Severity: state.SeverityNit, Body: "Опечатка.", Published: true},
			{ID: 4, File: "a.go", Severity: state.SeverityNit, Body: "Опечатка. И ещё…",
				Published: true},
		},
	}
}

func TestCommentForMarkerSurvivesEdits(t *testing.T) {
	r := linkReview()
	body := "**nit** Опечатка, поправил текст\n\n" + state.CommentMarker(1)
	if state.MarkedComment(body) != 1 ||
		state.WithoutMarker(body) != "**nit** Опечатка, поправил текст" {
		t.Fatalf("marker round trip: %d %q", state.MarkedComment(body), state.WithoutMarker(body))
	}
	r.Discussions = []state.Discussion{{ID: "d1", Author: "me", File: "a.go",
		Body: "**nit** Опечатка, поправил текст", Comment: state.MarkedComment(body)}}
	r.LinkComments()
	if r.Comments[0].ThreadID != "d1" || r.Comments[1].ThreadID != "" {
		t.Fatalf("the marker links #1 to d1: %+v", r.Comments)
	}
	r.Discussions[0].Comment, r.Discussions[0].Body = 0, "**nit** reworded again"
	if c := r.CommentFor(r.Discussions[0]); c == nil || c.ID != 1 {
		t.Fatalf("a stored link survives the marker being edited away: %+v", c)
	}
}

func TestDecideTouchesOnlyItsOwnResolve(t *testing.T) {
	r := linkReview()
	r.Discussions = []state.Discussion{
		{ID: "d1", Author: "me", File: "a.go", Body: "**nit** Опечатка.", Resolvable: true,
			Comment: 1},
		{ID: "d4", Author: "me", File: "a.go", Body: "**nit** Опечатка. И ещё…", Resolvable: true,
			Comment: 4},
	}
	now := time.Now()
	r.Comments[0].Resolved = true
	for _, v := range []string{state.VerdictResolve, state.VerdictOpen, state.VerdictNone} {
		if err := r.Decide("d1", v, "", now); err != nil {
			t.Fatal(err)
		}
		if !r.Comments[0].Resolved {
			t.Fatalf("%s must keep #1 resolved by gr comment resolve", v)
		}
	}
	if err := r.Decide("d4", state.VerdictResolve, "", now); err != nil || !r.Comments[1].Resolved {
		t.Fatalf("resolve resolves #4: %v %+v", err, r.Comments[1])
	}
	if th := r.ThreadState(r.Discussions[1]); !th.Decided() || !th.DecidedAt.Equal(now) {
		t.Fatalf("the decision records who and when: %+v", th)
	}
	if err := r.Decide("d4", state.VerdictOpen, "still wrong", now); err != nil ||
		r.Comments[1].Resolved {
		t.Fatalf("keep open takes back its own resolve: %v %+v", err, r.Comments[1])
	}
	if err := r.Decide("d4", "maybe", "", now); err == nil {
		t.Fatal("an unknown verdict is refused")
	}
}
