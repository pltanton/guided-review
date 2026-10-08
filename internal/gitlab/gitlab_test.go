package gitlab_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/pltanton/guided-review/internal/gitlab"
	"github.com/pltanton/guided-review/internal/state"
)

func TestParseMRURL(t *testing.T) {
	tests := []struct {
		in      string
		want    gitlab.MRRef
		wantErr bool
	}{
		{
			"https://gitlab.example.com/g/sub/proj/-/merge_requests/123",
			gitlab.MRRef{Host: "gitlab.example.com", Project: "g/sub/proj", IID: 123},
			false,
		},
		{
			"https://gitlab.example.com/g/proj/-/merge_requests/7/diffs?commit_id=abc",
			gitlab.MRRef{Host: "gitlab.example.com", Project: "g/proj", IID: 7},
			false,
		},
		{"https://gitlab.example.com/g/proj/-/issues/7", gitlab.MRRef{}, true},
		{"https://gitlab.example.com/g/proj/-/merge_requests/x", gitlab.MRRef{}, true},
		{"feature", gitlab.MRRef{}, true},
	}
	for _, tt := range tests {
		got, err := gitlab.ParseMRURL(tt.in)
		if (err != nil) != tt.wantErr || got != tt.want {
			t.Errorf("ParseMRURL(%q) = %+v, %v", tt.in, got, err)
		}
	}
}

func TestFetchMR(t *testing.T) {
	var gotArgs []string
	run := func(_ context.Context, args ...string) ([]byte, error) {
		gotArgs = args
		return []byte(
			`{"title":"Add guard","web_url":"https://h/g/p/-/merge_requests/7","source_branch":"feature",
			"diff_refs":{"base_sha":"b","start_sha":"s","head_sha":"h"}}`,
		), nil
	}
	mr, err := gitlab.FetchMR(
		context.Background(),
		run,
		gitlab.MRRef{Host: "h", Project: "g/p", IID: 7},
	)
	if err != nil {
		t.Fatal(err)
	}
	wantArgs := []string{"api", "--hostname", "h", "projects/g%2Fp/merge_requests/7"}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("args = %v, want %v", gotArgs, wantArgs)
	}
	want := gitlab.MR{
		Title:    "Add guard",
		WebURL:   "https://h/g/p/-/merge_requests/7",
		DiffRefs: gitlab.DiffRefs{BaseSHA: "b", StartSHA: "s", HeadSHA: "h"},
	}
	if mr != want {
		t.Fatalf("mr = %+v", mr)
	}
}

func TestFetchDiscussions(t *testing.T) {
	var gotArgs []string
	run := func(_ context.Context, args ...string) ([]byte, error) {
		gotArgs = args
		return []byte(`[{"id":"d1","notes":[
			{"body":"why float?","author":{"username":"alice"},"system":false,"resolvable":true,"resolved":false,
			 "position":{"new_path":"api/a.go","new_line":57,"old_path":"api/a.go","old_line":null}},
			{"body":"ok","author":{"username":"bob"},"system":false}]},
		 {"id":"d2","notes":[{"body":"added 1 commit","author":{"username":"bob"},"system":true}]},
		 {"id":"d5","notes":[{"body":"<!-- tracker-linkback -->","author":{"username":"tracker"},"system":false}]},
		 {"id":"d6","notes":[{"body":"<!-- review-bot fp=x -->\n**Stuck** approval","author":{"username":"ci"},"system":false}]}]
		[{"id":"d3","notes":[{"body":"general remark","author":{"username":"carol"},"system":false,"resolvable":true,"resolved":true}]},
		 {"id":"d4","notes":[{"body":"on removed line","author":{"username":"dan"},"system":false,
			 "position":{"new_path":"api/b.go","new_line":null,"old_path":"api/b.go","old_line":12}}]}]`), nil
	}
	got, err := gitlab.FetchDiscussions(
		context.Background(),
		run,
		gitlab.MRRef{Host: "h", Project: "g/p", IID: 7},
	)
	if err != nil {
		t.Fatal(err)
	}
	wantArgs := []string{
		"api",
		"--hostname",
		"h",
		"--paginate",
		"projects/g%2Fp/merge_requests/7/discussions?per_page=100",
	}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("args = %v", gotArgs)
	}
	want := []state.Discussion{
		{ID: "d1", Author: "alice", Body: "why float?", Replies: 1, File: "api/a.go", Line: 57,
			Resolvable: true, Notes: []state.Note{{Author: "alice", Body: "why float?"}, {Author: "bob", Body: "ok"}}},
		{ID: "d6", Author: "ci", Body: "**Stuck** approval", Notes: []state.Note{
			{Author: "ci", Body: "<!-- review-bot fp=x -->\n**Stuck** approval"}}},
		{ID: "d3", Author: "carol", Body: "general remark", Resolved: true, Resolvable: true,
			Notes: []state.Note{{Author: "carol", Body: "general remark"}}},
		{
			ID:      "d4",
			Author:  "dan",
			Body:    "on removed line",
			File:    "api/b.go",
			Line:    12,
			OldLine: true,
			Notes:   []state.Note{{Author: "dan", Body: "on removed line"}},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
}

func TestDraftJSON(t *testing.T) {
	ref := gitlab.MRRef{Host: "h", Project: "g/p", IID: 7}
	if got := ref.Path("/draft_notes"); got != "projects/g%2Fp/merge_requests/7/draft_notes" {
		t.Fatalf("Path = %q", got)
	}
	pos := &gitlab.Position{
		PositionType: "text", BaseSHA: "b", StartSHA: "s", HeadSHA: "h",
		OldPath: "a.go", NewPath: "a.go", NewLine: 12,
	}
	data, err := json.Marshal(gitlab.DraftNote{Note: "why?", Position: pos})
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatal(err)
	}
	p := body["position"].(map[string]any)
	if body["note"] != "why?" || p["new_line"] != float64(12) || p["base_sha"] != "b" {
		t.Fatalf("draft body %s", data)
	}
	if _, hasOld := p["old_line"]; hasOld {
		t.Fatalf("old_line must be omitted for an added line: %s", data)
	}
}

func TestCurrentUser(t *testing.T) {
	reply := `{"username":"alice"}`
	run := func(context.Context, ...string) ([]byte, error) { return []byte(reply), nil }
	if me, err := gitlab.CurrentUser(context.Background(), run, "h"); err != nil || me != "alice" {
		t.Fatalf("CurrentUser = %q, %v", me, err)
	}
	reply = `{}`
	if _, err := gitlab.CurrentUser(context.Background(), run, "h"); err == nil {
		t.Fatal("a reply without a username is an error")
	}
}
