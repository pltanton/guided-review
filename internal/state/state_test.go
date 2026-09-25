package state_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/aplotnikov/guided-review/internal/state"
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
	if got.Step("s1") == nil || got.Step("nope") != nil || got.StepIndex("s1") != 0 || got.File("a.go") == nil {
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
