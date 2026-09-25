package view

type Cell struct {
	Line  int
	Text  string
	Kind  RowKind
	Plain string
	Emph  [][2]int
	Moved bool
}

func cellOf(r Row, line int) Cell {
	return Cell{Line: line, Text: r.Text, Kind: r.Kind, Plain: r.Plain, Emph: r.Emph, Moved: r.Moved}
}

type SplitRow struct {
	Left, Right Cell
	Full        *Row
	File        string
	Line        int
	HunkStart   bool
	Hotspot     bool
}

func pairRows(rows []Row) []SplitRow {
	var out []SplitRow
	for i := 0; i < len(rows); {
		r := rows[i]
		switch r.Kind {
		case RowCode:
			out = append(out, SplitRow{
				Left:  cellOf(r, r.OldLine),
				Right: cellOf(r, r.Line),
				File:  r.File, Line: r.Line, HunkStart: r.HunkStart, Hotspot: r.Hotspot,
			})
			i++
		case RowRemoved, RowAdded:
			j := i
			for j < len(rows) && rows[j].Kind == RowRemoved {
				j++
			}
			k := j
			for k < len(rows) && rows[k].Kind == RowAdded && (k == j || !rows[k].HunkStart) {
				k++
			}
			rem, add := rows[i:j], rows[j:k]
			for n := range max(len(rem), len(add)) {
				sr := SplitRow{File: r.File, HunkStart: n == 0 && r.HunkStart}
				if n < len(rem) {
					sr.Left = cellOf(rem[n], rem[n].OldLine)
					sr.Line = rem[n].Line
				}
				if n < len(add) {
					sr.Right = cellOf(add[n], add[n].Line)
					sr.Line, sr.Hotspot = add[n].Line, add[n].Hotspot
				}
				out = append(out, sr)
			}
			i = k
		default:
			out = append(out, SplitRow{Full: &rows[i]})
			i++
		}
	}
	return out
}
