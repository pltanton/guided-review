package view

type Cell struct {
	Line     int
	Text     string
	Kind     RowKind
	Plain    string
	Emph     [][2]int
	Moved    bool
	Reformat bool
}

func cellOf(r Row, line int) Cell {
	return Cell{
		Line:  line,
		Text:  r.Text,
		Kind:  r.Kind,
		Plain: r.Plain,
		Emph:  r.Emph,
		Moved: r.Moved,
	}
}

type line struct {
	Row
	Left, Right Cell
	Pair        bool
}

func unifiedLines(rows []Row) []line {
	out := make([]line, len(rows))
	for i, r := range rows {
		out[i] = line{Row: r}
	}
	return out
}

func pairRows(rows []Row) []line {
	var out []line
	for i := 0; i < len(rows); {
		r := rows[i]
		switch r.Kind {
		case RowCode:
			out = append(
				out,
				line{Row: r, Left: cellOf(r, r.OldLine), Right: cellOf(r, r.Line), Pair: true},
			)
			i++
		case RowRemoved, RowAdded:
			j, k := changeRun(rows, i)
			rem, add := rows[i:j], rows[j:k]
			for n := range max(len(rem), len(add)) {
				l := line{Pair: true}
				if n < len(rem) {
					l.Row, l.Left = rem[n], cellOf(rem[n], rem[n].OldLine)
				}
				if n < len(add) {
					l.Row, l.Right = add[n], cellOf(add[n], add[n].Line)
				}
				l.HunkStart = n == 0 && r.HunkStart
				out = append(out, l)
			}
			i = k
		default:
			out = append(out, line{Row: r})
			i++
		}
	}
	return out
}

func changeRun(rows []Row, i int) (remEnd, addEnd int) {
	remEnd = i
	for remEnd < len(rows) && rows[remEnd].Kind == RowRemoved {
		remEnd++
	}
	addEnd = remEnd
	for addEnd < len(rows) && rows[addEnd].Kind == RowAdded &&
		(addEnd == remEnd || !rows[addEnd].HunkStart) {
		addEnd++
	}
	return remEnd, addEnd
}
