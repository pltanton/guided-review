package view

import (
	"reflect"
	"testing"
)

func TestPairRows(t *testing.T) {
	rows := []Row{
		{Kind: RowFile, File: "a.go", Text: "a.go"},
		{Kind: RowCode, File: "a.go", Line: 1, OldLine: 1, Text: "same"},
		{Kind: RowRemoved, File: "a.go", Line: 2, OldLine: 2, Text: "old2", HunkStart: true},
		{Kind: RowRemoved, File: "a.go", Line: 2, OldLine: 3, Text: "old3"},
		{Kind: RowAdded, File: "a.go", Line: 2, Text: "new2"},
		{Kind: RowNote, File: "a.go", Line: 2, Text: "note"},
		{Kind: RowAdded, File: "a.go", Line: 3, Text: "new3", HunkStart: true},
		{Kind: RowRemoved, File: "a.go", Line: 4, OldLine: 5, Text: "gone", HunkStart: true},
	}
	type pair struct {
		left, right string
		line        int
		hunk, full  bool
	}
	var got []pair
	for _, l := range pairRows(rows) {
		got = append(got, pair{l.Left.Text, l.Right.Text, l.Line, l.HunkStart, !l.Pair})
	}
	want := []pair{
		{"", "", 0, false, true},
		{"same", "same", 1, false, false},
		{"old2", "new2", 2, true, false},
		{"old3", "", 2, false, false},
		{"", "", 2, false, true},
		{"", "new3", 3, true, false},
		{"gone", "", 4, true, false},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %+v\nwant %+v", got, want)
	}
}
