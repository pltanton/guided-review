package state_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/pltanton/guided-review/internal/state"
)

func TestStoreRoundTrip(t *testing.T) {
	s := state.Store{Dir: t.TempDir()}
	if _, err := s.LoadCurrent(); !errors.Is(err, state.ErrNoReview) {
		t.Fatalf("LoadCurrent on empty store: %v", err)
	}
	r := &state.Review{
		ID: "mr-7", BaseSHA: "b", HeadSHA: "h",
		Files:    []state.File{{Path: "a.go", Status: "modified", Tier: state.TierCore, Added: 1}},
		Hotspots: []state.Hotspot{{Cat: "money", Q: "q", File: "a.go", Line: 2, Detail: "d"}},
		Comments: []state.Comment{{ID: 1, Suggestion: "\tif x {\n\t\treturn\n\t}"}},
	}
	if err := s.Save(r); err != nil {
		t.Fatal(err)
	}
	if err := s.SetCurrent("mr-7"); err != nil {
		t.Fatal(err)
	}
	got, err := s.LoadCurrent()
	if err != nil || !reflect.DeepEqual(got, r) {
		t.Fatalf("LoadCurrent = %+v, %v; want %+v", got, err, r)
	}
	if d, ok := got.Detail("a.go", 2); !ok || d != "d" || got.File("a.go") == nil {
		t.Fatal("lookup helpers")
	}
}

func TestFileOrder(t *testing.T) {
	r := &state.Review{Files: []state.File{
		{Path: "z.go", Tier: state.TierCore},
		{Path: "gen.pb.go", Tier: state.TierGenerated},
		{Path: "b.go", Tier: state.TierCore, Order: 2},
		{Path: "a.go", Tier: state.TierCore},
		{Path: "wire.go", Tier: state.TierBoilerplate},
		{Path: "c.go", Tier: state.TierCore, Order: 1},
	}}
	var got []string
	for _, f := range r.FileOrder() {
		got = append(got, f.Path)
	}
	want := []string{"c.go", "b.go", "a.go", "z.go", "wire.go", "gen.pb.go"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("FileOrder = %v, want %v", got, want)
	}
}

func TestParseLines(t *testing.T) {
	tests := []struct {
		in         string
		start, end int
		wantErr    bool
	}{
		{"", 0, 0, false},
		{"57", 57, 57, false},
		{"40-92", 40, 92, false},
		{"0", 0, 0, true},
		{"9-3", 0, 0, true},
		{"a-b", 0, 0, true},
	}
	for _, tt := range tests {
		s, e, err := state.ParseLines(tt.in)
		if (err != nil) != tt.wantErr || s != tt.start || e != tt.end {
			t.Errorf("ParseLines(%q) = %d, %d, %v", tt.in, s, e, err)
		}
	}
}

func TestStoreKeysAndList(t *testing.T) {
	dir := t.TempDir()
	a := state.Store{Dir: dir, Key: "aaa"}
	b := state.Store{Dir: dir, Key: "bbb"}
	for _, id := range []string{"mr-1", "mr-2"} {
		if err := a.Save(&state.Review{ID: id}); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.SetCurrent("mr-1"); err != nil {
		t.Fatal(err)
	}
	if err := b.SetCurrent("mr-2"); err != nil {
		t.Fatal(err)
	}
	if id, _ := a.Current(); id != "mr-1" {
		t.Fatalf("a.Current = %q", id)
	}
	if id, _ := b.Current(); id != "mr-2" {
		t.Fatalf("b.Current = %q", id)
	}
	ids, err := a.List()
	if err != nil || !reflect.DeepEqual(ids, []string{"mr-1", "mr-2"}) {
		t.Fatalf("List = %v, %v", ids, err)
	}
	if err := a.ClearCurrent(); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Current(); !errors.Is(err, state.ErrNoReview) {
		t.Fatalf("after ClearCurrent: %v", err)
	}
	if !state.IsCurrentFile("current-aaa") || state.IsCurrentFile("state.yaml") {
		t.Fatal("IsCurrentFile")
	}
}

func TestDetail(t *testing.T) {
	r := state.Review{
		Hotspots:    []state.Hotspot{{Cat: "money", Q: "?", File: "a.go", Line: 7, Detail: "risk"}},
		Annotations: []state.Annotation{{File: "a.go", Line: 3, To: 5, Kind: "note", Text: "t"}},
	}
	if _, ok := r.Detail("a.go", 5); ok {
		t.Fatal("a note without a detail has none")
	}
	if d, _ := r.Detail("a.go", 7); d != "risk" {
		t.Fatalf("hotspot detail = %q", d)
	}
}

func TestMyThreadsSkipTheSummary(t *testing.T) {
	r := state.Review{MR: &state.MR{Me: "me"}, Discussions: []state.Discussion{
		{ID: "sum", Author: "me", Resolvable: true, Body: "**Guided review: changes requested**\n\nComments: 1 minor."},
		{ID: "d1", Author: "me", Resolvable: true, File: "a.go", Line: 3, Body: "why?"},
	}}
	got := r.MyThreads()
	if len(got) != 1 || got[0].ID != "d1" {
		t.Fatalf("the posted summary is not a thread to decide: %+v", got)
	}
}
