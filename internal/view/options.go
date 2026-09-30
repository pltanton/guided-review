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

func (m *model) optionPills() (line string, spans [][2]int) {
	x := 0
	for i, o := range m.answerOptions() {
		pill := keyStyle.Render(fmt.Sprintf(" %d", i+1)) + buttonStyle.Render(" "+o+" ")
		w := ansi.StringWidth(pill)
		spans = append(spans, [2]int{x, x + w})
		line += pill + " "
		x += w + 1
	}
	hint := "· " + m.keys().key("message") + " own answer"
	if m.step != nil {
		hint = "· alt+number " + hint
	}
	return line + dimStyle.Render(hint), spans
}

func (m *model) answer(i int) {
	if opts := m.answerOptions(); i >= 0 && i < len(opts) {
		m.emit(inbox.Event{Kind: inbox.KindMessage, Text: opts[i]})
	}
}

func (m *model) optionKey(k string) (int, bool) {
	digit := strings.TrimPrefix(k, "alt+")
	if len(digit) != 1 || digit < "1" || digit > "9" || digit == k && m.step != nil {
		return 0, false
	}
	return int(digit[0] - '1'), len(m.answerOptions()) > 0
}

func (m *model) optionAt(x int) int {
	_, spans := m.optionPills()
	for i, sp := range spans {
		if x >= sp[0] && x < sp[1] {
			return i
		}
	}
	return -1
}
