package state_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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
		Files: []state.File{{Path: "a.go", Status: "modified", Tier: state.TierCore, Added: 1}},
		Steps: []state.Step{{ID: "s1", Title: "t", Kind: "logic", Status: state.StatusPending,
			Hunks: []state.StepHunk{{File: "a.go", Lines: "1-3"}}}},
		Current: "s1",
	}
	if err := s.Save(r); err != nil {
		t.Fatal(err)
	}
	if !s.Exists("mr-7") {
		t.Fatal("Exists = false after Save")
	}
	if err := s.SetCurrent("mr-7"); err != nil {
		t.Fatal(err)
	}
	got, err := s.LoadCurrent()
	if err != nil || !reflect.DeepEqual(got, r) {
		t.Fatalf("LoadCurrent = %+v, %v; want %+v", got, err, r)
	}
	if got.Step("s1") == nil || got.Step("nope") != nil || got.StepIndex("s1") != 0 ||
		got.File("a.go") == nil {
		t.Fatal("lookup helpers")
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

func TestHotspotFile(t *testing.T) {
	st := state.Step{Hunks: []state.StepHunk{{File: "a.go", Lines: "1-20"}, {File: "b.go", Lines: "30-60"}}}
	tests := []struct {
		name string
		h    state.Hotspot
		want string
	}{
		{"explicit", state.Hotspot{File: "a.go", Line: 40}, "a.go"},
		{"inside b's range", state.Hotspot{Line: 40}, "b.go"},
		{"outside every range falls back to the first file", state.Hotspot{Line: 25}, "a.go"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, _ := st.HotspotFile(tt.h); got != tt.want {
				t.Fatalf("HotspotFile(%+v) = %q, want %q", tt.h, got, tt.want)
			}
		})
	}
	one := state.Step{Hunks: []state.StepHunk{{File: "a.go"}}}
	if got, sure := one.HotspotFile(state.Hotspot{Line: 99}); got != "a.go" || !sure {
		t.Fatalf("a single-file step owns its hotspots, got %q", got)
	}
}

func TestLoadMovesOldDetailsIntoNotes(t *testing.T) {
	s := state.Store{Dir: t.TempDir()}
	old := `id: mr-1
steps:
  - id: s1
    hunks: [{file: a.go}]
    hotspots: [{cat: money, q: "?", file: a.go, line: 7, detail: plan text}]
    annotations: [{file: a.go, line: 3, to: 5, kind: note, text: t}]
    details:
      - {file: a.go, line: 5, text: note detail}
      - {file: a.go, line: 7, text: edited hotspot detail}
      - {file: a.go, line: 9, text: nothing there}
`
	if err := os.MkdirAll(s.ReviewDir("mr-1"), 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.ReviewDir("mr-1"), state.FileName)
	if err := os.WriteFile(path, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	r, err := s.Load("mr-1")
	if err != nil {
		t.Fatal(err)
	}
	st := r.Step("s1")
	if st.Annotations[0].Detail != "note detail" ||
		st.Hotspots[0].Detail != "edited hotspot detail" {
		t.Fatalf("details must move into their notes: %+v", st)
	}
	if err := s.Save(r); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); strings.Contains(string(data), "details:") {
		t.Fatalf("details must not be written back:\n%s", data)
	}
}

func TestStepDetail(t *testing.T) {
	st := state.Step{
		Hunks:       []state.StepHunk{{File: "a.go"}},
		Hotspots:    []state.Hotspot{{Cat: "money", Q: "?", Line: 7}},
		Annotations: []state.Annotation{{File: "a.go", Line: 3, To: 5, Kind: "note", Text: "t"}},
	}
	if _, ok := st.Detail("a.go", 5); ok {
		t.Fatal("a note without a detail has none")
	}
	if !st.SetDetail("a.go", 5, "why") || !st.SetDetail("a.go", 7, "risk") ||
		st.SetDetail("a.go", 4, "x") {
		t.Fatal("a detail lands only on the last line of a note or on a hotspot")
	}
	if d, _ := st.Detail("a.go", 5); d != "why" || st.Annotations[0].Detail != "why" {
		t.Fatalf("annotation detail: %+v", st.Annotations)
	}
	if d, _ := st.Detail("a.go", 7); d != "risk" || st.Hotspots[0].Detail != "risk" {
		t.Fatalf("hotspot detail: %+v", st.Hotspots)
	}
}
