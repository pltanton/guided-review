package view

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	minSimilarity = 0.5
	maxWordDiff   = 400
	minMovedLines = 3
	minFoldLines  = 3
)

var tokenRe = regexp.MustCompile(`\w+|\s+|[^\w\s]`)

type token struct {
	text       string
	start, end int
}

func tokens(s string) []token {
	var out []token
	for _, loc := range tokenRe.FindAllStringIndex(s, -1) {
		start := utf8.RuneCountInString(s[:loc[0]])
		text := s[loc[0]:loc[1]]
		out = append(out, token{text, start, start + utf8.RuneCountInString(text)})
	}
	return out
}

func wordDiff(a, b string) (ea, eb [][2]int, sim float64) {
	if a == b {
		return nil, nil, 1
	}
	if len(a) > maxWordDiff || len(b) > maxWordDiff {
		return nil, nil, 0
	}
	ta, tb := tokens(a), tokens(b)
	lcs := make([][]int, len(ta)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(tb)+1)
	}
	for i := len(ta) - 1; i >= 0; i-- {
		for j := len(tb) - 1; j >= 0; j-- {
			if ta[i].text == tb[j].text {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	keepA, keepB := make([]bool, len(ta)), make([]bool, len(tb))
	common := 0
	for i, j := 0, 0; i < len(ta) && j < len(tb); {
		switch {
		case ta[i].text == tb[j].text:
			keepA[i], keepB[j] = true, true
			common += utf8.RuneCountInString(ta[i].text)
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			i++
		default:
			j++
		}
	}
	total := utf8.RuneCountInString(a) + utf8.RuneCountInString(b)
	if total == 0 {
		return nil, nil, 1
	}
	return spans(ta, keepA), spans(tb, keepB), 2 * float64(common) / float64(total)
}

func spans(ts []token, keep []bool) [][2]int {
	var out [][2]int
	for i, t := range ts {
		if keep[i] || strings.TrimSpace(t.text) == "" {
			continue
		}
		if n := len(out); n > 0 && out[n-1][1] >= t.start-1 &&
			onlySpaceBetween(ts, out[n-1][1], t.start) {
			out[n-1][1] = t.end
			continue
		}
		out = append(out, [2]int{t.start, t.end})
	}
	return out
}

func onlySpaceBetween(ts []token, from, to int) bool {
	for _, t := range ts {
		if t.start >= from && t.end <= to && strings.TrimSpace(t.text) != "" {
			return false
		}
	}
	return true
}

func changeRuns(rows []Row, visit func(i, j, k int)) {
	for i := 0; i < len(rows); {
		if kind := rows[i].Kind; kind != RowRemoved && kind != RowAdded {
			i++
			continue
		}
		j, k := changeRun(rows, i)
		visit(i, j, k)
		i = k
	}
}

func markIntraline(rows []Row) {
	changeRuns(rows, func(i, j, k int) {
		for p := range min(j-i, k-j) {
			a, b := &rows[i+p], &rows[j+p]
			if a.Reformat {
				continue
			}
			ea, eb, sim := wordDiff(a.Plain, b.Plain)
			if sim >= minSimilarity && sim < 1 {
				a.Emph, b.Emph = nonNil(ea), nonNil(eb)
			}
		}
	})
}

func nonNil(s [][2]int) [][2]int {
	if s == nil {
		return [][2]int{}
	}
	return s
}

func moveKey(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func markMoved(rows []Row) {
	var removedRuns [][]int
	changeRuns(rows, func(i, j, _ int) {
		if j-i < minMovedLines {
			return
		}
		run := make([]int, 0, j-i)
		for n := i; n < j; n++ {
			run = append(run, n)
		}
		removedRuns = append(removedRuns, run)
	})
	for _, run := range removedRuns {
		for s := 0; s+minMovedLines <= len(run); {
			start, length := findAdded(rows, run[s:])
			if length < minMovedLines {
				s++
				continue
			}
			for k := range length {
				r, a := &rows[run[s+k]], &rows[start+k]
				r.Moved, a.Moved = true, true
				r.MovedTo = fmt.Sprintf("%s:%d", rows[start].File, rows[start].Line)
				a.MovedTo = fmt.Sprintf("%s:%d", rows[run[s]].File, rows[run[s]].OldLine)
			}
			s += length
		}
	}
}

func findAdded(rows []Row, rem []int) (start, length int) {
	for i := range rows {
		if rows[i].Kind != RowAdded || rows[i].Moved || moveKey(rows[i].Plain) == "" {
			continue
		}
		n := 0
		for n < len(rem) && i+n < len(rows) && rows[i+n].Kind == RowAdded && !rows[i+n].Moved &&
			moveKey(rows[i+n].Plain) == moveKey(rows[rem[n]].Plain) {
			n++
		}
		if n > length {
			start, length = i, n
		}
	}
	return start, length
}

func foldRemoved(rows []Row, unfolded map[string]bool) []Row {
	out := make([]Row, 0, len(rows))
	for i := 0; i < len(rows); {
		if rows[i].Kind != RowRemoved {
			out = append(out, rows[i])
			i++
			continue
		}
		j := i
		for j < len(rows) && rows[j].Kind == RowRemoved {
			j++
		}
		out = append(out, foldRun(rows[i:j], unfolded)...)
		i = j
	}
	return out
}

func foldRun(run []Row, unfolded map[string]bool) []Row {
	key := fmt.Sprintf("%s:%d:%d", run[0].File, run[0].Line, run[0].OldLine)
	if unfolded[key] {
		open := make([]Row, len(run))
		for i, r := range run {
			r.FoldKey = key
			open[i] = r
		}
		return open
	}
	fold := func(rows []Row, text string) Row {
		return Row{Kind: RowFold, File: rows[0].File, Line: rows[0].Line, OldLine: rows[0].OldLine,
			HunkStart: rows[0].HunkStart, FoldCount: len(rows), FoldKey: key, Text: text}
	}
	allMoved, allReformat := true, true
	for _, r := range run {
		allMoved = allMoved && r.Moved
		allReformat = allReformat && r.Reformat
	}
	switch {
	case allReformat:
		return nil
	case allMoved:
		return []Row{fold(run, fmt.Sprintf("↕ %d lines moved to %s", len(run), run[0].MovedTo))}
	}
	var kept, loose []Row
	for _, r := range run {
		if r.Emph != nil {
			kept = append(kept, r)
		} else {
			loose = append(loose, r)
		}
	}
	if len(loose) < minFoldLines {
		return run
	}
	f := fold(loose, fmt.Sprintf("▸ %d removed lines hidden", len(loose)))
	f.HunkStart = run[0].HunkStart
	return append([]Row{f}, kept...)
}
