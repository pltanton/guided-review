package view

import (
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/pltanton/guided-review/internal/diff"
	"github.com/pltanton/guided-review/internal/state"
)

func TestWordDiff(t *testing.T) {
	ea, eb, sim := wordDiff("x := compute(a, b)", "x := compute(a, c)")
	if !reflect.DeepEqual(ea, [][2]int{{16, 17}}) || !reflect.DeepEqual(eb, [][2]int{{16, 17}}) {
		t.Fatalf("emph = %v %v", ea, eb)
	}
	if sim < 0.8 {
		t.Fatalf("sim = %v", sim)
	}
	if _, _, sim := wordDiff("return nil", "for _, key := range order {"); sim >= 0.5 {
		t.Fatalf("dissimilar lines sim = %v", sim)
	}
}

func TestMarkIntraline(t *testing.T) {
	rows := []Row{
		{Kind: RowRemoved, Plain: "if a > 0 {"},
		{Kind: RowRemoved, Plain: "totally different"},
		{Kind: RowAdded, Plain: "if a >= 0 {"},
		{Kind: RowAdded, Plain: "brand new stuff here"},
	}
	markIntraline(rows)
	if rows[0].Emph == nil || rows[2].Emph == nil {
		t.Fatalf("similar pair not marked: %+v", rows)
	}
	if rows[1].Emph != nil || rows[3].Emph != nil {
		t.Fatalf("dissimilar pair marked: %+v", rows)
	}
}

func TestMarkMoved(t *testing.T) {
	rows := []Row{
		{Kind: RowRemoved, File: "a.go", OldLine: 10, Plain: "func helper() {"},
		{Kind: RowRemoved, File: "a.go", OldLine: 11, Plain: "\treturn 42"},
		{Kind: RowRemoved, File: "a.go", OldLine: 12, Plain: "}"},
		{Kind: RowCode, File: "a.go", Line: 10, Plain: "x"},
		{Kind: RowAdded, File: "b.go", Line: 5, Plain: "func helper() {"},
		{Kind: RowAdded, File: "b.go", Line: 6, Plain: "    return 42"},
		{Kind: RowAdded, File: "b.go", Line: 7, Plain: "}"},
	}
	markMoved(rows)
	for _, i := range []int{0, 1, 2, 4, 5, 6} {
		if !rows[i].Moved {
			t.Fatalf("row %d not moved: %+v", i, rows[i])
		}
	}
	if rows[0].MovedTo != "b.go:5" || rows[4].MovedTo != "a.go:10" {
		t.Fatalf("moved refs: %q %q", rows[0].MovedTo, rows[4].MovedTo)
	}
}

func TestReformatHunk(t *testing.T) {
	src := fakeSource{
		files: map[string]diff.File{"a.go": {Path: "a.go", Hunks: []diff.Hunk{
			{OldStart: 2, OldLines: 1, NewStart: 2, NewLines: 2, Lines: []diff.Line{
				{
					Kind: '-',
					Text: "call(a,b)",
				}, {Kind: '+', Text: "call(a,"}, {Kind: '+', Text: "  b)"}}},
		}}},
		lines: map[string][]string{"a.go": {"x", "call(a,", "  b)", "y"}},
	}
	rows, err := buildRows(src, state.Step{Hunks: []state.StepHunk{{File: "a.go"}}}, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if (r.Kind == RowAdded || r.Kind == RowRemoved) && !r.Reformat {
			t.Fatalf("row not marked reformat: %+v", r)
		}
	}
}

func TestFoldRemoved(t *testing.T) {
	rows := []Row{
		{Kind: RowFile, File: "a.go", Text: "a.go"},
		{
			Kind:      RowRemoved,
			File:      "a.go",
			Line:      5,
			OldLine:   5,
			Plain:     "old1",
			Text:      "old1",
			HunkStart: true,
		},
		{Kind: RowRemoved, File: "a.go", Line: 5, OldLine: 6, Plain: "old2", Text: "old2"},
		{Kind: RowRemoved, File: "a.go", Line: 5, OldLine: 7, Plain: "old3", Text: "old3"},
		{Kind: RowAdded, File: "a.go", Line: 5, Plain: "new", Text: "new"},
		{
			Kind:      RowRemoved,
			File:      "a.go",
			Line:      9,
			OldLine:   11,
			Plain:     "one",
			Text:      "one",
			HunkStart: true,
		},
		{Kind: RowCode, File: "a.go", Line: 9, Plain: "keep", Text: "keep"},
	}
	out := foldRemoved(rows, nil)
	var kinds []RowKind
	for _, r := range out {
		kinds = append(kinds, r.Kind)
	}
	want := []RowKind{RowFile, RowFold, RowAdded, RowRemoved, RowCode}
	if !reflect.DeepEqual(kinds, want) {
		t.Fatalf("kinds = %v, want %v", kinds, want)
	}
	if out[1].FoldCount != 3 || !out[1].HunkStart || out[1].FoldKey == "" {
		t.Fatalf("fold row: %+v", out[1])
	}
	open := foldRemoved(rows, map[string]bool{out[1].FoldKey: true})
	if len(open) != len(rows) {
		t.Fatalf("unfolded: %d rows, want %d", len(open), len(rows))
	}
}

func TestRenames(t *testing.T) {
	tests := []struct {
		a, b     string
		from, to string
		ok       bool
	}{
		{"x := calcFee(a)", "x := computeFee(a)", "calcFee", "computeFee", true},
		{"return calcFee(calcFee(a))", "return computeFee(computeFee(a))", "calcFee", "computeFee", true},
		{"ok := true", "ok := false", "", "", false},
		{"n := 1", "n := 2", "", "", false},
		{"a := b + c", "z := b + d", "", "", false},
	}
	for _, tt := range tests {
		from, to, ok := renamePair(tt.a, tt.b)
		if from != tt.from || to != tt.to || ok != tt.ok {
			t.Errorf("renamePair(%q, %q) = %q, %q, %v", tt.a, tt.b, from, to, ok)
		}
	}
	row := func(kind RowKind, text string) Row { return Row{Kind: kind, Plain: text, Text: text} }
	rows := []Row{
		row(RowRemoved, "a := calcFee(x)"), row(RowRemoved, "b := calcFee(y)"),
		row(RowAdded, "a := computeFee(x)"), row(RowAdded, "b := computeFee(y)"),
		row(RowRemoved, "ok := true"), row(RowAdded, "ok := false"),
	}
	markRenames(rows)
	if !rows[0].RenameHide || rows[2].RenamedFrom != "calcFee" || rows[4].RenameHide {
		t.Fatalf("renames: %+v", rows)
	}
	lone := []Row{row(RowRemoved, "a := calcFee(x)"), row(RowAdded, "a := computeFee(x)")}
	if markRenames(lone); lone[0].RenameHide {
		t.Fatal("a single swapped identifier stays a normal change")
	}
	out := ansi.Strip(renderCode(cellOf(Row{Kind: RowAdded, Line: 3, Text: "a := computeFee(x)",
		Plain: "a := computeFee(x)", RenamedFrom: "calcFee"}, 3), false))
	if !strings.HasPrefix(out, "⇄") || !strings.HasSuffix(out, "← was calcFee") {
		t.Fatalf("rename line: %q", out)
	}
}
