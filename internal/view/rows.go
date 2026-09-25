package view

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"

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
	RowNote
	RowFold
	RowSpacer
)

type Row struct {
	Kind      RowKind
	File      string
	Line      int
	OldLine   int
	Text      string
	Hotspot   bool
	HunkStart bool
	NoteKind  string
	NoteLabel string
	NoteHead  bool
	Dim       bool

	Plain     string
	Emph      [][2]int
	Moved     bool
	MovedTo   string
	Reformat  bool
	FoldCount int
	FoldKey   string
	Ref       int
	GapFrom   int
	GapTo     int
	FileInfo  string
}

type Note struct {
	Ref   int
	File  string
	Line  int
	Kind  string
	Label string
	Text  string
	Dim   bool
	Focus bool
}

type Source interface {
	FileDiff(path string) (diff.File, error)
	Lines(path string) ([]string, error)
}

func buildRows(
	src Source,
	st state.Step,
	context int,
	notes []Note,
	reveal map[string][][2]int,
) ([]Row, error) {
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
		var fileNotes []Note
		for _, n := range notes {
			if n.File == "" || n.File == sh.File {
				fileNotes = append(fileNotes, n)
			}
		}
		if len(rows) > 0 {
			rows = append(rows, Row{Kind: RowSpacer, File: sh.File})
		}
		rows = append(
			rows,
			Row{Kind: RowFile, File: sh.File, Text: sh.File, FileInfo: fileInfo(fd)},
		)
		rows = append(
			rows,
			fileRows(
				fd,
				lines,
				start,
				end,
				context,
				hotspotLines(st, sh.File),
				fileNotes,
				reveal[sh.File],
			)...)
	}
	markIntraline(rows)
	markMoved(rows)
	return rows, nil
}

func fileInfo(fd diff.File) string {
	added, deleted := fd.Stat()
	status := string(fd.Status)
	if fd.OldPath != "" && fd.OldPath != fd.Path {
		status = "renamed from " + fd.OldPath
	}
	if fd.Binary {
		status += ", binary"
	}
	return fmt.Sprintf("+%d −%d · %s", added, deleted, status)
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

func hunkBefore(h diff.Hunk, n int) bool {
	if h.NewLines == 0 {
		return h.NewStart < n
	}
	return h.NewStart+h.NewLines-1 < n
}

func fileRows(
	fd diff.File,
	lines []string,
	start, end, context int,
	hot map[int]bool,
	notes []Note,
	reveal [][2]int,
) []Row {
	added := map[int]bool{}
	reformat := map[int]bool{}
	removed := map[int][]Row{}
	hunkStart := map[int]bool{}
	for _, h := range fd.Hunks {
		anchor := removalAnchor(h)
		hunkStart[anchor] = true
		onlyFormat := isReformat(h)
		n, old := h.NewStart, h.OldStart
		for _, l := range h.Lines {
			if l.Kind == '+' {
				added[n], reformat[n] = true, onlyFormat
				n++
				continue
			}
			text := expandTabs(l.Text)
			removed[anchor] = append(
				removed[anchor],
				Row{
					Kind:     RowRemoved,
					File:     fd.Path,
					OldLine:  old,
					Text:     text,
					Plain:    text,
					Reformat: onlyFormat,
				},
			)
			old++
		}
	}
	notesAt := map[int][]Note{}
	for _, n := range notes {
		notesAt[n.Line] = append(notesAt[n.Line], n)
	}

	var rows []Row
	if len(lines) == 0 {
		for _, n := range slices.Sorted(maps.Keys(removed)) {
			for i, r := range removed[n] {
				r.Line, r.HunkStart = 1, i == 0
				rows = append(rows, r)
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
	for _, n := range notes {
		if n.Focus && n.Line > 0 {
			windows = append(windows, [2]int{n.Line - context, n.Line + context})
		}
	}
	windows = append(windows, reveal...)
	oldLine := func(n int) int {
		delta := 0
		for _, h := range fd.Hunks {
			if hunkBefore(h, n) {
				delta += h.NewLines - h.OldLines
			}
		}
		return n - delta
	}
	gap := func(from, to int) Row {
		return Row{Kind: RowGap, File: fd.Path, Text: "⋯", GapFrom: from, GapTo: to}
	}
	prevEnd := 0
	for _, w := range mergeWindows(windows, len(lines)) {
		if w[0] > prevEnd+1 {
			rows = append(rows, gap(prevEnd+1, w[0]-1))
		}
		prevEnd = w[1]
		for n := w[0]; n <= w[1]+1; n++ {
			first := hunkStart[n]
			for _, r := range removed[n] {
				r.Line, r.HunkStart = min(n, len(lines)), first
				rows = append(rows, r)
				first = false
			}
			if n > w[1] {
				break
			}
			row := Row{
				Kind:      RowCode,
				File:      fd.Path,
				Line:      n,
				Text:      lines[n-1],
				Plain:     ansi.Strip(lines[n-1]),
				Hotspot:   hot[n],
				HunkStart: first,
			}
			if added[n] {
				row.Kind, row.Reformat = RowAdded, reformat[n]
			} else {
				row.OldLine = oldLine(n)
			}
			rows = append(rows, row)
			for _, note := range notesAt[n] {
				rows = append(
					rows,
					Row{
						Kind:      RowNote,
						File:      fd.Path,
						Line:      n,
						Text:      note.Text,
						NoteKind:  note.Kind,
						NoteLabel: note.Label,
						Dim:       note.Dim,
						Ref:       note.Ref,
					},
				)
			}
		}
	}
	if prevEnd > 0 && prevEnd < len(lines) {
		rows = append(rows, gap(prevEnd+1, len(lines)))
	}
	return rows
}

func isReformat(h diff.Hunk) bool {
	var before, after strings.Builder
	for _, l := range h.Lines {
		squashed := strings.Join(strings.Fields(l.Text), "")
		if l.Kind == '+' {
			after.WriteString(squashed)
		} else {
			before.WriteString(squashed)
		}
	}
	return h.OldLines > 0 && h.NewLines > 0 && before.Len() > 0 &&
		before.String() == after.String()
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
