package view

import (
	"fmt"

	"github.com/charmbracelet/x/ansi"
)

const (
	hscrollStep = 8
	minCodeText = 10
)

func codeParts(c Cell, hot bool) (gutter, cont, text string) {
	marker, num, text := " ", "    ", c.Text
	if c.Line != 0 {
		num = fmt.Sprintf("%4d", c.Line)
	}
	switch c.Kind {
	case RowAdded:
		marker = addStyle.Render("+")
	case RowRemoved:
		marker = delStyle.Render("-")
	}
	switch {
	case c.Moved:
		marker, text = dimStyle.Render("↕"), dimStyle.Render(c.Plain)
	case c.Reformat:
		marker = dimStyle.Render("≈")
	case c.Emph != nil:
		text = emphasize(c.Text, c.Emph, c.Kind)
	}
	if c.RenamedFrom != "" {
		marker = gapStyle.Render("⇄")
		text += dimStyle.Render("  ← was " + c.RenamedFrom)
	}
	if hot {
		marker = hotStyle.Render("⚑")
	}
	sep := dimStyle.Render(" │ ")
	if c.Mark != "" {
		sep = " " + noteKinds[c.Mark].tone.fg().Render("┃") + " "
	}
	return marker + dimStyle.Render(num) + sep, "     " + sep, text
}

func codeAvail(w int) int { return max(w-codePrefix, minCodeText) }

func (m *model) codeRows(c Cell, hot bool, w int) (rows []string) {
	gutter, cont, text := codeParts(c, hot)
	avail, width := codeAvail(w), ansi.StringWidth(text)
	if !m.nowrap {
		for from := 0; from == 0 || from < width; from += avail {
			lead := cont
			if from == 0 {
				lead = gutter
			}
			rows = append(rows, lead+ansi.Cut(text, from, from+avail))
		}
		return rows
	}
	if width-m.hscroll > avail {
		cut := ansi.Cut(text, m.hscroll, m.hscroll+avail-1)
		return []string{gutter + cut + dimStyle.Render("›")}
	}
	return []string{gutter + ansi.Cut(text, m.hscroll, width)}
}

func (m *model) cellHeight(c Cell, w int) int {
	if m.nowrap || c.Line == 0 && c.Text == "" {
		return 1
	}
	_, _, text := codeParts(c, false)
	avail := codeAvail(w)
	return max((ansi.StringWidth(text)+avail-1)/avail, 1)
}

func isCode(k RowKind) bool { return k == RowCode || k == RowAdded || k == RowRemoved }

func (m *model) splitWidths(w int) (left, right int) {
	left = (w - 2) / 2
	return left, w - left - 1
}

func (m *model) lineHeight(i int) int {
	if m.nowrap || i < 0 || i >= len(m.lines) {
		return 1
	}
	l, w := m.lines[i], m.mainWidth()
	switch {
	case l.Pair && m.useSplit():
		lw, rw := m.splitWidths(w)
		return max(m.cellHeight(l.Left, lw), m.cellHeight(l.Right, rw))
	case isCode(l.Kind):
		return m.cellHeight(cellOf(l.Row, l.Line), w)
	}
	return 1
}

func (m *model) topFor(i, rows int) int {
	h := m.lineHeight(i)
	for i > 0 {
		next := m.lineHeight(i - 1)
		if h+next > rows {
			break
		}
		h += next
		i--
	}
	return i
}

func (m *model) lastShown() int {
	body, h := m.bodyHeight(), 0
	i := m.offset
	for ; i < len(m.lines); i++ {
		if h += m.lineHeight(i); h > body {
			break
		}
	}
	return max(i-1, m.offset)
}

func (m *model) renderRows(i, w int) []string {
	l := m.lines[i]
	if l.Pair && m.useSplit() {
		return m.renderSplit(l, w)
	}
	if !isCode(l.Kind) {
		return []string{renderUnified(m.animate(l.Row))}
	}
	c := cellOf(l.Row, l.Line)
	if l.Kind == RowRemoved {
		c.Line = 0
	}
	return m.codeRows(c, l.Hotspot, w)
}

func (m *model) underlineWord(rows []string, from, to, w int) {
	avail := codeAvail(w)
	for k := range rows {
		start := m.hscroll
		if !m.nowrap {
			start = k * avail
		}
		f, t := max(from-start, 0), min(to-start, avail)
		if f < t {
			rows[k] = underline(rows[k], codePrefix+f, codePrefix+t)
		}
	}
}

func (m *model) maxHScroll() int {
	over := 0
	w := m.mainWidth()
	measure := func(c Cell, cw int) {
		if c.Line == 0 && c.Text == "" {
			return
		}
		_, _, text := codeParts(c, false)
		over = max(over, ansi.StringWidth(text)-codeAvail(cw))
	}
	lw, rw := m.splitWidths(w)
	for _, l := range m.lines {
		switch {
		case l.Pair && m.useSplit():
			measure(l.Left, lw)
			measure(l.Right, rw)
		case isCode(l.Kind):
			measure(cellOf(l.Row, l.Line), w)
		}
	}
	return over
}

func (m *model) scrollSideways(d int) {
	if !m.nowrap {
		m.status = "long lines wrap: " + m.keys().key("wrap") + " or :set nowrap to scroll sideways"
		return
	}
	m.hscroll = max(0, min(m.hscroll+d, m.maxHScroll()))
}

func (m *model) setWrap(on bool) {
	m.nowrap, m.hscroll = !on, 0
	m.clamp()
	m.status = "long lines wrap"
	if !on {
		m.status = "long lines are cut at › · " + m.keys().key("scroll-left") + "/" +
			m.keys().key("scroll-right") + " scroll sideways"
	}
}

func (m *model) followCol() {
	if !m.nowrap || m.useSplit() {
		return
	}
	avail := codeAvail(m.mainWidth())
	switch {
	case m.col < m.hscroll:
		m.hscroll = m.col
	case m.col >= m.hscroll+avail-1:
		m.hscroll = m.col - avail + 2
	}
}

func (m *model) markShown() {
	body, h := m.bodyHeight(), 0
	for i := m.offset; i < len(m.lines); i++ {
		if h += m.lineHeight(i); h > body {
			return
		}
		if !m.lineCut(i) {
			m.markSeen(m.lines[i])
		}
	}
}

func (m *model) lineCut(i int) bool {
	if !m.nowrap {
		return false
	}
	l, w := m.lines[i], m.mainWidth()
	cut := func(c Cell, cw int) bool {
		_, _, text := codeParts(c, false)
		return ansi.StringWidth(text)-m.hscroll > codeAvail(cw)
	}
	switch {
	case l.Pair && m.useSplit():
		lw, rw := m.splitWidths(w)
		return cut(l.Left, lw) || cut(l.Right, rw)
	case isCode(l.Kind):
		return cut(cellOf(l.Row, l.Line), w)
	}
	return false
}
