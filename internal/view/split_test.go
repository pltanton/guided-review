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
	got := pairRows(rows)
	want := []SplitRow{
		{Full: &rows[0]},
		{Left: Cell{Line: 1, Text: "same", Kind: RowCode}, Right: Cell{Line: 1, Text: "same", Kind: RowCode}, File: "a.go", Line: 1},
		{Left: Cell{Line: 2, Text: "old2", Kind: RowRemoved}, Right: Cell{Line: 2, Text: "new2", Kind: RowAdded}, File: "a.go", Line: 2, HunkStart: true},
		{Left: Cell{Line: 3, Text: "old3", Kind: RowRemoved}, File: "a.go", Line: 2},
		{Full: &rows[5]},
		{Right: Cell{Line: 3, Text: "new3", Kind: RowAdded}, File: "a.go", Line: 3, HunkStart: true},
		{Left: Cell{Line: 5, Text: "gone", Kind: RowRemoved}, File: "a.go", Line: 4, HunkStart: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %+v\nwant %+v", got, want)
	}
}
