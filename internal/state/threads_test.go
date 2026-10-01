package state_test

import (
	"testing"

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

func TestCommentForExactBody(t *testing.T) {
	r := linkReview()
	d := state.Discussion{ID: "d4", Author: "me", File: "a.go", Body: "**nit** Опечатка. И ещё…"}
	if c := r.CommentFor(d); c == nil || c.ID != 4 {
		t.Fatalf("a longer body must not link to #1: %+v", c)
	}
	d.Body = "`a.go:3` **nit** Опечатка.\n\n```suggestion:-0+0\nx\n```"
	if c := r.CommentFor(d); c == nil || c.ID != 1 {
		t.Fatalf("a general note with a suggestion links by its text: %+v", c)
	}
	d.Author = "alice"
	if c := r.CommentFor(d); c != nil {
		t.Fatalf("someone else's thread is not a gr comment: %+v", c)
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
