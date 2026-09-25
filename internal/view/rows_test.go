package view

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

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
	rows, err := BuildRows(src, state.Step{Hunks: []state.StepHunk{{File: "a.go"}}}, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := "F0:a.go\n~0:⋯\n 2:L2\n->3:old3\n+3:L3\n 4:L4\n~0:⋯\n 11:L11\n 12:L12\n->13:gone\n 13:L13\n~0:⋯\n"
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
	rows, err := BuildRows(src, st, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := "F0:a.go\n~0:⋯\n+>5:L5\n!6:L6\n~0:⋯\n"
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
	rows, err := BuildRows(src, state.Step{Hunks: []state.StepHunk{{File: "gone.go"}}}, 3, nil)
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

func TestBuildRowsNotesAndOldLines(t *testing.T) {
	src := fakeSource{
		files: map[string]diff.File{"a.go": {Path: "a.go", Hunks: []diff.Hunk{
			{OldStart: 2, OldLines: 1, NewStart: 2, NewLines: 2, Lines: []diff.Line{{Kind: '-', Text: "old2"}, {Kind: '+', Text: "L2"}, {Kind: '+', Text: "L3"}}},
			{OldStart: 20, OldLines: 0, NewStart: 21, NewLines: 1, Lines: []diff.Line{{Kind: '+', Text: "L21"}}},
		}}},
		lines: map[string][]string{"a.go": numbered(30)},
	}
	st := state.Step{Hunks: []state.StepHunk{{File: "a.go", Lines: "2-3"}}}
	notes := []Note{
		{File: "a.go", Line: 3, Kind: "note", Text: "retry wrapper", Focus: true},
		{File: "a.go", Line: 21, Kind: "spec", Text: "spec says 409", Focus: true},
		{File: "a.go", Line: 28, Kind: "mr", Text: "@alice: far away"},
	}
	rows, err := BuildRows(src, st, 1, notes)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, r := range rows {
		got = append(got, fmt.Sprintf("%d/%d %d %s", r.OldLine, r.Line, r.Kind, r.Text))
	}
	want := []string{
		fmt.Sprintf("0/0 %d a.go", RowFile),
		fmt.Sprintf("1/1 %d L1", RowCode),
		fmt.Sprintf("2/2 %d old2", RowRemoved),
		fmt.Sprintf("0/2 %d L2", RowAdded),
		fmt.Sprintf("0/3 %d L3", RowAdded),
		fmt.Sprintf("0/3 %d retry wrapper", RowNote),
		fmt.Sprintf("3/4 %d L4", RowCode),
		fmt.Sprintf("0/0 %d ⋯", RowGap),
		fmt.Sprintf("19/20 %d L20", RowCode),
		fmt.Sprintf("0/21 %d L21", RowAdded),
		fmt.Sprintf("0/21 %d spec says 409", RowNote),
		fmt.Sprintf("20/22 %d L22", RowCode),
		fmt.Sprintf("0/0 %d ⋯", RowGap),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestWholeFileNoteOutsideHunks(t *testing.T) {
	src := fakeSource{
		files: map[string]diff.File{"a.go": {Path: "a.go", Hunks: []diff.Hunk{
			{NewStart: 30, NewLines: 1, Lines: []diff.Line{{Kind: '+', Text: "L30"}}},
		}}},
		lines: map[string][]string{"a.go": numbered(40)},
	}
	rows, err := BuildRows(src, state.Step{Hunks: []state.StepHunk{{File: "a.go"}}}, 1, []Note{{File: "a.go", Line: 5, Kind: "note", Text: "far note", Focus: true}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(summary(rows), "far note") {
		t.Fatalf("note outside hunk windows is lost:\n%s", summary(rows))
	}
}

func TestGapsAndReveal(t *testing.T) {
	src := fakeSource{
		files: map[string]diff.File{"a.go": {Path: "a.go", Hunks: []diff.Hunk{
			{NewStart: 30, NewLines: 1, Lines: []diff.Line{{Kind: '+', Text: "L30"}}},
		}}},
		lines: map[string][]string{"a.go": numbered(40)},
	}
	st := state.Step{Hunks: []state.StepHunk{{File: "a.go"}}}
	rows, err := BuildRows(src, st, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	var gaps [][2]int
	for _, r := range rows {
		if r.Kind == RowGap {
			gaps = append(gaps, [2]int{r.GapFrom, r.GapTo})
		}
	}
	if !reflect.DeepEqual(gaps, [][2]int{{1, 28}, {32, 40}}) {
		t.Fatalf("gaps = %v", gaps)
	}
	rows, err = BuildRowsWith(src, st, 1, nil, map[string][][2]int{"a.go": {{1, 28}}})
	if err != nil {
		t.Fatal(err)
	}
	if rows[1].Kind != RowCode || rows[1].Line != 1 {
		t.Fatalf("revealed rows must start at line 1: %+v", rows[1])
	}
}

func TestFileHeaders(t *testing.T) {
	src := fakeSource{
		files: map[string]diff.File{
			"a.go": {Path: "a.go", Status: diff.Modified, Hunks: []diff.Hunk{{NewStart: 1, NewLines: 1, Lines: []diff.Line{{Kind: '+', Text: "L1"}}}}},
			"b.go": {Path: "b.go", OldPath: "old/b.go", Status: diff.Renamed, Hunks: []diff.Hunk{{OldStart: 1, OldLines: 1, NewStart: 1, NewLines: 1, Lines: []diff.Line{{Kind: '-', Text: "x"}, {Kind: '+', Text: "L1"}}}}},
		},
		lines: map[string][]string{"a.go": numbered(1), "b.go": numbered(1)},
	}
	rows, err := BuildRows(src, state.Step{Hunks: []state.StepHunk{{File: "a.go"}, {File: "b.go"}}}, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	var kinds []RowKind
	for _, r := range rows {
		kinds = append(kinds, r.Kind)
	}
	want := []RowKind{RowFile, RowAdded, RowSpacer, RowFile, RowRemoved, RowAdded}
	if !reflect.DeepEqual(kinds, want) {
		t.Fatalf("kinds = %v, want %v", kinds, want)
	}
	if rows[0].FileInfo != "+1 −0 · modified" || rows[3].FileInfo != "+1 −1 · renamed from old/b.go" {
		t.Fatalf("file info: %q / %q", rows[0].FileInfo, rows[3].FileInfo)
	}
	if got := ansi.Strip(renderUnified(rows[3])); !strings.Contains(got, "b.go") || !strings.Contains(got, "renamed from old/b.go") {
		t.Fatalf("header render: %q", got)
	}
}
