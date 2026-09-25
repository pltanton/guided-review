package view

import (
	"maps"
	"slices"

	"github.com/aplotnikov/guided-review/internal/diff"
	"github.com/aplotnikov/guided-review/internal/state"
)

type RowKind int

const (
	RowFile RowKind = iota
	RowGap
	RowCode
	RowAdded
	RowRemoved
)

type Row struct {
	Kind      RowKind
	File      string
	Line      int
	Text      string
	Hotspot   bool
	HunkStart bool
}

type Source interface {
	FileDiff(path string) (diff.File, error)
	Lines(path string) ([]string, error)
}

func BuildRows(src Source, st state.Step, context int) ([]Row, error) {
	var rows []Row
	for _, sh := range st.Hunks {
		fd, err := src.FileDiff(sh.File)
		if err != nil {
			return nil, err
		}
		lines, err := src.Lines(sh.File)
		if err != nil {
			return nil, err
		}
		start, end, err := state.ParseLines(sh.Lines)
		if err != nil {
			return nil, err
		}
		rows = append(rows, Row{Kind: RowFile, File: sh.File, Text: sh.File})
		rows = append(rows, fileRows(fd, lines, start, end, context, hotspotLines(st, sh.File))...)
	}
	return rows, nil
}

func hotspotLines(st state.Step, file string) map[int]bool {
	out := map[int]bool{}
	for _, h := range st.Hotspots {
		if h.Line > 0 && (h.File == "" || h.File == file) {
			out[h.Line] = true
		}
	}
	return out
}

func removalAnchor(h diff.Hunk) int {
	if h.NewLines == 0 {
		return h.NewStart + 1
	}
	return h.NewStart
}

func fileRows(fd diff.File, lines []string, start, end, context int, hot map[int]bool) []Row {
	added := map[int]bool{}
	removed := map[int][]string{}
	hunkStart := map[int]bool{}
	for _, h := range fd.Hunks {
		anchor := removalAnchor(h)
		hunkStart[anchor] = true
		n := h.NewStart
		for _, l := range h.Lines {
			if l.Kind == '+' {
				added[n] = true
				n++
			} else {
				removed[anchor] = append(removed[anchor], expandTabs(l.Text))
			}
		}
	}

	var rows []Row
	if len(lines) == 0 {
		for _, n := range slices.Sorted(maps.Keys(removed)) {
			for i, t := range removed[n] {
				rows = append(rows, Row{Kind: RowRemoved, File: fd.Path, Line: 1, Text: t, HunkStart: i == 0})
			}
		}
		return rows
	}

	var windows [][2]int
	if start > 0 {
		windows = [][2]int{{start - context, end + context}}
	} else {
		for _, h := range fd.Hunks {
			windows = append(windows, [2]int{h.NewStart - context, h.NewEnd() + context})
		}
	}
	for i, w := range mergeWindows(windows, len(lines)) {
		if i > 0 {
			rows = append(rows, Row{Kind: RowGap, File: fd.Path, Text: "⋯"})
		}
		for n := w[0]; n <= w[1]+1; n++ {
			first := hunkStart[n]
			for _, t := range removed[n] {
				rows = append(rows, Row{Kind: RowRemoved, File: fd.Path, Line: min(n, len(lines)), Text: t, HunkStart: first})
				first = false
			}
			if n > w[1] {
				break
			}
			kind := RowCode
			if added[n] {
				kind = RowAdded
			}
			rows = append(rows, Row{Kind: kind, File: fd.Path, Line: n, Text: lines[n-1], Hotspot: hot[n], HunkStart: first})
		}
	}
	return rows
}

func mergeWindows(ws [][2]int, maxLine int) [][2]int {
	for i := range ws {
		ws[i][0] = max(ws[i][0], 1)
		ws[i][1] = min(ws[i][1], maxLine)
	}
	ws = slices.DeleteFunc(ws, func(w [2]int) bool { return w[0] > w[1] })
	slices.SortFunc(ws, func(a, b [2]int) int { return a[0] - b[0] })
	var out [][2]int
	for _, w := range ws {
		if n := len(out); n > 0 && w[0] <= out[n-1][1]+1 {
			out[n-1][1] = max(out[n-1][1], w[1])
			continue
		}
		out = append(out, w)
	}
	return out
}
