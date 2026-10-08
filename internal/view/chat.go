package view

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func (m *model) chatFocusHint() string {
	km := m.keys()
	return fmt.Sprintf("%s/%s move · %s select · %s copy · esc back", km.key("down"), km.key("up"),
		km.key("select"), km.key("yank"))
}

func (m *model) chatWindow(rows []chatRow, h, w int) []string {
	if m.chatFocus && len(rows) > 0 {
		m.chatCursor = max(0, min(m.chatCursor, len(rows)-1))
		switch start, end := windowRange(len(rows), h, m.chatTop); {
		case m.chatCursor >= end:
			m.chatTop = len(rows) - m.chatCursor - 1
		case m.chatCursor < start:
			m.chatTop = max(len(rows)-m.chatCursor-h, 0)
		}
	}
	start, end := windowRange(len(rows), h, m.chatTop)
	out := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		line := rows[i].text
		switch {
		case m.chatFocus && i == m.chatCursor:
			line = paint(fit(line, w), cursorTone)
		case m.chatSelected(i):
			line = paint(fit(line, w), selectTone)
		}
		out = append(out, line)
	}
	return out
}

func (m *model) chatSelected(i int) bool {
	return m.chatVisual && i >= min(m.chatAnchor, m.chatCursor) &&
		i <= max(m.chatAnchor, m.chatCursor)
}

func (m *model) chatBox() (rows []chatRow, x, y, h int, ok bool) {
	switch {
	case m.review == nil || m.threads || m.preview != "":
		return nil, 0, 0, 0, false
	case m.step == nil:
		w := min(max(m.width-2, 20), intakeWidth)
		top := m.intakeTop(w)
		h = max(m.height-len(top)-len(m.intakePrompt(w))-1, 1)
		return m.chatRows(w, false), m.inputRowX(), len(top) + 1, h, true
	case m.chatWidth() > 0:
		w := m.chatWidth() - 2
		h = m.height - len(m.bottomLines()) - sideChatTop - len(m.sidePrompt(w))
		return m.chatRows(w, true), m.width - m.chatWidth() + 2, sideChatTop, h, true
	}
	rows = m.chatRows(m.width, false)
	return rows, 0, m.height - len(m.bottomLines()) + 1, messageLines, len(rows) > 0
}

func (m *model) chatRowAt(x, y int) (int, bool) {
	rows, x0, y0, h, ok := m.chatBox()
	if !ok || x < x0 || y < y0 || y >= y0+h {
		return 0, false
	}
	start, end := windowRange(len(rows), h, m.chatTop)
	i := start + y - y0
	return i, i < end
}

func (m *model) focusChat() {
	rows, _, _, _, ok := m.chatBox()
	if !ok || len(rows) == 0 {
		m.status = "no chat to select from yet"
		return
	}
	m.chatFocus, m.chatVisual, m.chatCursor = true, false, len(rows)-1
}

func (m *model) handleChatKey(msg tea.KeyMsg) tea.Cmd {
	km := m.keys()
	name := ""
	if i, ok := km.byKey[msg.String()]; ok {
		name = km.actions[i].Name
	}
	switch name {
	case "down":
		m.chatCursor++
	case "up":
		m.chatCursor = max(m.chatCursor-1, 0)
	case "bottom":
		m.chatCursor = 1 << 20
	case "select":
		m.chatVisual, m.chatAnchor = !m.chatVisual, m.chatCursor
	case "yank":
		m.yankChat()
	case "back", "chat":
		if m.chatVisual {
			m.chatVisual = false
		} else {
			m.chatFocus = false
		}
	case "quit":
		return tea.Quit
	}
	rows, _, _, _, _ := m.chatBox()
	m.chatCursor = max(0, min(m.chatCursor, len(rows)-1))
	return nil
}

func (m *model) yankChat() {
	rows, _, _, _, _ := m.chatBox()
	lo, hi := m.chatCursor, m.chatCursor
	if m.chatVisual {
		lo, hi = min(m.chatAnchor, m.chatCursor), max(m.chatAnchor, m.chatCursor)
	}
	var out []string
	last := [2]int{-1, -1}
	for i := lo; i <= hi && i < len(rows); i++ {
		r := rows[i]
		if r.msg < 0 {
			continue
		}
		if !m.chatVisual {
			for _, s := range rows {
				if s.msg == r.msg && [2]int{s.msg, s.hard} != last {
					out, last = append(out, ansi.Strip(s.source)), [2]int{s.msg, s.hard}
				}
			}
			break
		}
		if at := [2]int{r.msg, r.hard}; at != last {
			out, last = append(out, ansi.Strip(r.source)), at
		}
	}
	if len(out) == 0 {
		m.status = "nothing to copy here"
		return
	}
	if m.copyText(strings.Join(out, "\n")) {
		m.chatVisual = false
		m.status = copiedStatus(len(out))
	}
}

func (m *model) chatMouse(msg tea.MouseMsg) bool {
	switch {
	case msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft:
		if m.optionAt(msg.X, msg.Y) >= 0 || m.onChatInput(msg.X, msg.Y) {
			return false
		}
		i, ok := m.chatRowAt(msg.X, msg.Y)
		m.chatDrag, m.chatFrom = ok, i
		return ok
	case msg.Action == tea.MouseActionMotion && m.chatDrag:
		if i, ok := m.chatRowAt(msg.X, msg.Y); ok && (i != m.chatFrom || m.chatVisual) {
			m.chatFocus, m.chatVisual, m.chatAnchor, m.chatCursor = true, true, m.chatFrom, i
		}
		return true
	case msg.Action == tea.MouseActionRelease && m.chatDrag:
		m.chatDrag = false
		if m.chatVisual {
			m.yankChat()
			m.chatFocus, m.chatVisual = false, false
		}
		return true
	}
	return false
}

func copiedStatus(n int) string {
	if n == 1 {
		return "copied 1 line"
	}
	return fmt.Sprintf("copied %d lines", n)
}
