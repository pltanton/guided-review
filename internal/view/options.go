package view

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/pltanton/guided-review/internal/inbox"
)

func (m *model) answerOptions() []string {
	if m.review == nil || len(m.review.Messages) == 0 {
		return nil
	}
	last := m.review.Messages[len(m.review.Messages)-1]
	for _, e := range m.events {
		if e.Time.After(last.Time) {
			return nil
		}
	}
	return last.Options
}

type pillSpan struct{ row, from, to int }

func (m *model) optionPills(w int) (lines []string, spans []pillSpan) {
	var pills []string
	total := -1
	for i, o := range m.answerOptions() {
		k := fmt.Sprint(i + 1)
		pill := keyStyle.Render(" "+k) + buttonStyle.Render(" "+o+" ")
		pills = append(pills, pill)
		total += ansi.StringWidth(pill) + 1
	}
	if total <= w {
		line, x := "", 0
		for _, p := range pills {
			pw := ansi.StringWidth(p)
			spans = append(spans, pillSpan{0, x, x + pw})
			line += p + " "
			x += pw + 1
		}
		lines = []string{line}
	} else {
		for i, p := range pills {
			lines = append(lines, p)
			spans = append(spans, pillSpan{i, 0, ansi.StringWidth(p)})
		}
	}
	return append(lines, dimStyle.Render(m.keys().key("message")+" own answer")), spans
}

func (m *model) pillsAt() (x, y, w int, ok bool) {
	if len(m.answerOptions()) == 0 || m.composing {
		return 0, 0, 0, false
	}
	block := func(w int) int { lines, _ := m.optionPills(w); return len(lines) }
	switch {
	case m.step == nil:
		_, w, _ = m.cardFrame(intakeWidth)
		return m.inputRowX(), m.height - 2 - block(w), w, m.err == nil
	case m.chatWidth() > 0:
		w = m.chatWidth() - 2
		return m.width - m.chatWidth() + 2, m.height - len(m.bottomLines()) - block(w), w, true
	case len(m.chatLines(m.width, false)) == 0:
		return 0, 0, 0, false
	}
	return 0, m.height - 1 - block(m.width), m.width, true
}

func (m *model) answer(i int) {
	if opts := m.answerOptions(); i >= 0 && i < len(opts) {
		m.emit(inbox.Event{Kind: inbox.KindMessage, Text: opts[i]})
	}
}

func (m *model) optionKey(k string) (int, bool) {
	digit := strings.TrimPrefix(k, "alt+")
	if len(digit) != 1 || digit < "1" || digit > "9" {
		return 0, false
	}
	return int(digit[0] - '1'), len(m.answerOptions()) > 0
}

func (m *model) optionAt(x, y int) int {
	x0, y0, w, ok := m.pillsAt()
	if !ok {
		return -1
	}
	_, spans := m.optionPills(w)
	for i, sp := range spans {
		if y == y0+sp.row && x-x0 >= sp.from && x-x0 < sp.to {
			return i
		}
	}
	return -1
}
