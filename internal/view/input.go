package view

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/aplotnikov/guided-review/internal/inbox"
)

func (m *model) startCompose(kind string) {
	if m.review == nil || m.step == nil && kind != inbox.KindMessage {
		return
	}
	m.composing, m.composeKind, m.input = true, kind, nil
}

func (m *model) handleCompose(msg tea.KeyMsg) {
	switch msg.Type {
	case tea.KeyEsc:
		m.composing, m.input = false, nil
	case tea.KeyEnter:
		text := strings.TrimSpace(string(m.input))
		m.composing, m.input = false, nil
		if text == "" {
			return
		}
		e := inbox.Event{Kind: m.composeKind, Text: text}
		if m.composeKind == inbox.KindMessage && m.visual {
			if file, lines, ok := m.selection(); ok {
				e.File, e.Lines = file, lines
			}
		}
		m.emit(e)
	case tea.KeyBackspace:
		if n := len(m.input); n > 0 {
			m.input = m.input[:n-1]
		}
	case tea.KeyCtrlU:
		m.input = nil
	case tea.KeyRunes, tea.KeySpace:
		m.input = append(m.input, msg.Runes...)
	}
}

func (m *model) explain() {
	file, lines, ok := m.selection()
	if !ok {
		m.status = "nothing to explain here: put the cursor on code"
		return
	}
	m.emit(inbox.Event{Kind: inbox.KindExplain, File: file, Lines: lines})
}

func (m *model) emit(e inbox.Event) {
	if m.review == nil || m.step == nil && e.Kind != inbox.KindMessage {
		return
	}
	if e.Step == "" && m.step != nil {
		e.Step = m.step.ID
	}
	if err := m.send(e); err != nil {
		m.err = err
		return
	}
	m.visual = false
	m.status = "sent to agent: " + e.Kind
	if m.review != nil && m.store.Dir != "" {
		m.events, _ = inbox.All(m.store.ReviewDir(m.review.ID))
	}
	if m.step != nil && m.src != nil {
		m.rebuild(false)
	}
}

func (m *model) selection() (file, lines string, ok bool) {
	cur := m.current()
	if cur.File == "" || cur.Line == 0 {
		return "", "", false
	}
	if !m.visual {
		return cur.File, fmt.Sprint(cur.Line), true
	}
	lo, hi := min(m.anchor, m.cursor), max(m.anchor, m.cursor)
	first, last := 0, 0
	for i := lo; i <= hi && i < len(m.list); i++ {
		it := m.list[i]
		if it.File != cur.File || it.Line == 0 {
			continue
		}
		if first == 0 || it.Line < first {
			first = it.Line
		}
		last = max(last, it.Line)
	}
	if first == last {
		return cur.File, fmt.Sprint(first), true
	}
	return cur.File, fmt.Sprintf("%d-%d", first, last), true
}

func (m *model) selected(i int) bool {
	return m.visual && i >= min(m.anchor, m.cursor) && i <= max(m.anchor, m.cursor)
}
