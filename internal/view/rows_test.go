package view

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/aplotnikov/guided-review/internal/diff"
	"github.com/aplotnikov/guided-review/internal/state"
)

type fakeSource struct {
	files map[string]diff.File
	lines map[string][]string
}

func (f fakeSource) FileDiff(path string) (diff.File, error) {
	d, ok := f.files[path]
	if !ok {
		return diff.File{}, fmt.Errorf("%s: not in diff", path)
	}
	return d, nil
}

func (f fakeSource) Lines(path string) ([]string, error) { return f.lines[path], nil }

func numbered(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("L%d", i+1)
	}
	return out
}

func summary(rows []Row) string {
	var b strings.Builder
	for _, r := range rows {
		mark := map[RowKind]string{RowFile: "F", RowGap: "~", RowCode: " ", RowAdded: "+", RowRemoved: "-"}[r.Kind]
		if r.Hotspot {
			mark = "!"
		}
		if r.HunkStart {
			mark += ">"
		}
		fmt.Fprintf(&b, "%s%d:%s\n", mark, r.Line, r.Text)
	}
	return b.String()
}

func TestBuildRowsWholeFile(t *testing.T) {
	src := fakeSource{
		files: map[string]diff.File{"a.go": {Path: "a.go", Hunks: []diff.Hunk{
			{NewStart: 3, NewLines: 1, Lines: []diff.Line{{Kind: '-', Text: "old3"}, {Kind: '+', Text: "L3"}}},
			{NewStart: 12, NewLines: 0, Lines: []diff.Line{{Kind: '-', Text: "gone"}}},
		}}},
		lines: map[string][]string{"a.go": numbered(20)},
	}
	rows, err := BuildRows(src, state.Step{Hunks: []state.StepHunk{{File: "a.go"}}}, 1)
	if err != nil {
		t.Fatal(err)
	}
	want := "F0:a.go\n 2:L2\n->3:old3\n+3:L3\n 4:L4\n~0:⋯\n 11:L11\n 12:L12\n->13:gone\n 13:L13\n"
	if got := summary(rows); got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

func TestBuildRowsRangeAndHotspot(t *testing.T) {
	src := fakeSource{
		files: map[string]diff.File{"a.go": {Path: "a.go", Hunks: []diff.Hunk{
			{NewStart: 5, NewLines: 2, Lines: []diff.Line{{Kind: '+', Text: "L5"}, {Kind: '+', Text: "L6"}}},
		}}},
		lines: map[string][]string{"a.go": numbered(10)},
	}
	st := state.Step{
		Hunks:    []state.StepHunk{{File: "a.go", Lines: "5-6"}},
		Hotspots: []state.Hotspot{{Cat: "money", Q: "?", Line: 6}},
	}
	rows, err := BuildRows(src, st, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := "F0:a.go\n+>5:L5\n!6:L6\n"
	if got := summary(rows); got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

func TestBuildRowsDeletedFile(t *testing.T) {
	src := fakeSource{
		files: map[string]diff.File{"gone.go": {Path: "gone.go", Status: diff.Deleted, Hunks: []diff.Hunk{
			{OldStart: 1, OldLines: 2, Lines: []diff.Line{{Kind: '-', Text: "a"}, {Kind: '-', Text: "b"}}},
		}}},
		lines: map[string][]string{},
	}
	rows, err := BuildRows(src, state.Step{Hunks: []state.StepHunk{{File: "gone.go"}}}, 3)
	if err != nil {
		t.Fatal(err)
	}
	var kinds []RowKind
	for _, r := range rows {
		kinds = append(kinds, r.Kind)
	}
	if !reflect.DeepEqual(kinds, []RowKind{RowFile, RowRemoved, RowRemoved}) || !rows[1].HunkStart {
		t.Fatalf("rows: %s", summary(rows))
	}
}
