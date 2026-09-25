package gitlab_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/aplotnikov/guided-review/internal/gitlab"
)

func TestParseMRURL(t *testing.T) {
	tests := []struct {
		in      string
		want    gitlab.MRRef
		wantErr bool
	}{
		{"https://gitlab.example.com/g/sub/proj/-/merge_requests/123", gitlab.MRRef{Host: "gitlab.example.com", Project: "g/sub/proj", IID: 123}, false},
		{"https://gitlab.example.com/g/proj/-/merge_requests/7/diffs?commit_id=abc", gitlab.MRRef{Host: "gitlab.example.com", Project: "g/proj", IID: 7}, false},
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
		return []byte(`{"title":"Add guard","web_url":"https://h/g/p/-/merge_requests/7","source_branch":"feature",
			"diff_refs":{"base_sha":"b","start_sha":"s","head_sha":"h"}}`), nil
	}
	mr, err := gitlab.FetchMR(context.Background(), run, gitlab.MRRef{Host: "h", Project: "g/p", IID: 7})
	if err != nil {
		t.Fatal(err)
	}
	wantArgs := []string{"api", "--hostname", "h", "projects/g%2Fp/merge_requests/7"}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("args = %v, want %v", gotArgs, wantArgs)
	}
	want := gitlab.MR{Title: "Add guard", WebURL: "https://h/g/p/-/merge_requests/7", SourceBranch: "feature",
		DiffRefs: gitlab.DiffRefs{BaseSHA: "b", StartSHA: "s", HeadSHA: "h"}}
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
		 {"id":"d2","notes":[{"body":"added 1 commit","author":{"username":"bob"},"system":true}]}]
		[{"id":"d3","notes":[{"body":"general remark","author":{"username":"carol"},"system":false,"resolvable":true,"resolved":true}]},
		 {"id":"d4","notes":[{"body":"on removed line","author":{"username":"dan"},"system":false,
			 "position":{"new_path":"api/b.go","new_line":null,"old_path":"api/b.go","old_line":12}}]}]`), nil
	}
	got, err := gitlab.FetchDiscussions(context.Background(), run, gitlab.MRRef{Host: "h", Project: "g/p", IID: 7})
	if err != nil {
		t.Fatal(err)
	}
	wantArgs := []string{"api", "--hostname", "h", "--paginate", "projects/g%2Fp/merge_requests/7/discussions?per_page=100"}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Fatalf("args = %v", gotArgs)
	}
	want := []gitlab.Discussion{
		{ID: "d1", Author: "alice", Body: "why float?", Replies: 1, File: "api/a.go", Line: 57},
		{ID: "d3", Author: "carol", Body: "general remark", Resolved: true},
		{ID: "d4", Author: "dan", Body: "on removed line", File: "api/b.go", Line: 12},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
}
