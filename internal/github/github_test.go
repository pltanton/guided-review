package github_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/pltanton/guided-review/internal/github"
)

func TestParsePRURL(t *testing.T) {
	tests := []struct {
		give    string
		want    github.PRRef
		wantErr bool
	}{
		{"https://github.com/o/r/pull/12", github.PRRef{Host: "github.com", Project: "o/r", Number: 12}, false},
		{"https://ghe.corp/o/r/pull/3/files", github.PRRef{Host: "ghe.corp", Project: "o/r", Number: 3}, false},
		{"https://gitlab.com/g/p/-/merge_requests/7", github.PRRef{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.give, func(t *testing.T) {
			got, err := github.ParsePRURL(tt.give)
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Fatalf("ParsePRURL(%q) = %+v, %v; want %+v", tt.give, got, err, tt.want)
			}
		})
	}
}

func TestFetchDiscussions(t *testing.T) {
	var args []string
	run := func(_ context.Context, a ...string) ([]byte, error) {
		args = a
		return []byte(`{"data":{"repository":{"pullRequest":{
			"reviewThreads":{"nodes":[
				{"id":"t1","isResolved":false,"path":"a.go","line":12,"diffSide":"RIGHT",
				 "comments":{"totalCount":3,"nodes":[{"databaseId":71,"body":"why float?","author":{"login":"alice"}},
				   {"databaseId":72,"body":"it is money","author":{"login":"bob"}}]}},
				{"id":"t2","isResolved":true,"path":"b.go","line":null,"originalLine":4,"diffSide":"RIGHT",
				 "comments":{"totalCount":1,"nodes":[{"body":"<!-- bot -->","author":{"login":"ci"}}]}},
				{"id":"t3","isResolved":false,"path":"c.go","line":null,"originalLine":9,"diffSide":"RIGHT",
				 "comments":{"totalCount":1,"nodes":[{"body":"outdated","author":{"login":"bob"}}]}}]},
			"comments":{"nodes":[{"id":"c1","body":"LGTM overall","author":{"login":"carol"}}]}}}}}`), nil
	}
	ds, err := github.FetchDiscussions(context.Background(), run,
		github.PRRef{Host: "github.com", Project: "o/r", Number: 5})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(args, " "), "number=5") || args[2] != "github.com" {
		t.Fatalf("query args %q", args)
	}
	want := []github.Discussion{
		{ID: "t1", Author: "alice", Body: "why float?", Replies: 2, File: "a.go", Line: 12,
			ReplyTo: 71, Resolvable: true, Notes: []github.Note{
				{Author: "alice", Body: "why float?"}, {Author: "bob", Body: "it is money"}}},
		{ID: "t3", Author: "bob", Body: "outdated", File: "c.go", Line: 9, OldLine: true, Resolvable: true,
			Notes: []github.Note{{Author: "bob", Body: "outdated"}}},
		{ID: "c1", Author: "carol", Body: "LGTM overall"},
	}
	if len(ds) != len(want) {
		t.Fatalf("got %+v", ds)
	}
	for i := range want {
		if !reflect.DeepEqual(ds[i], want[i]) {
			t.Fatalf("discussion %d = %+v, want %+v", i, ds[i], want[i])
		}
	}
}

func TestCurrentUser(t *testing.T) {
	reply := `{"login":"alice"}`
	run := func(context.Context, ...string) ([]byte, error) { return []byte(reply), nil }
	if me, err := github.CurrentUser(context.Background(), run, "github.com"); err != nil || me != "alice" {
		t.Fatalf("CurrentUser = %q, %v", me, err)
	}
	reply = `{}`
	if _, err := github.CurrentUser(context.Background(), run, "github.com"); err == nil ||
		!strings.Contains(err.Error(), "gh auth login") {
		t.Fatalf("a reply without a login is an error, got %v", err)
	}
}
