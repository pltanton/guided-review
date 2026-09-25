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
	m.composing, m.composeKind, m.input, m.inputPos = true, kind, nil, 0
	m.composeRef, m.anchorFile, m.anchorLines = 0, "", ""
	if kind != inbox.KindMessage {
		return
	}
	cur := m.current()
	switch {
	case m.visual:
		m.anchorFile, m.anchorLines, _ = m.selection()
	case cur.Ref > 0:
		m.composeRef, m.anchorFile, m.anchorLines = cur.Ref, cur.File, fmt.Sprint(cur.Line)
	case cur.File != "" && cur.Line > 0:
		m.anchorFile, m.anchorLines = cur.File, fmt.Sprint(cur.Line)
	}
}

func (m *model) startEdit() {
	ref := m.current().Ref
	if ref == 0 || m.review == nil {
		m.status = "put the cursor on one of your comments to edit it"
		return
	}
	for _, c := range m.review.Comments {
		if c.ID == ref {
			m.composing, m.composeKind, m.composeRef = true, inbox.KindEdit, ref
			m.anchorFile, m.anchorLines = "", ""
			m.input = []rune(c.Body)
			m.inputPos = len(m.input)
			return
		}
	}
}

func (m *model) handleCompose(msg tea.KeyMsg) tea.Cmd {
	if m.cmdMode != 0 {
		switch msg.Type {
		case tea.KeyEnter:
			return m.submitCmd()
		case tea.KeyTab:
			m.complete()
			return nil
		case tea.KeyUp:
			m.historyMove(-1)
			return nil
		case tea.KeyDown:
			m.historyMove(1)
			return nil
		case tea.KeyEsc:
			m.composing, m.input, m.cmdMode = false, nil, 0
			return nil
		}
	}
	switch msg.Type {
	case tea.KeyEsc:
		m.composing, m.input = false, nil
	case tea.KeyEnter:
		text := strings.TrimSpace(string(m.input))
		m.composing, m.input = false, nil
		if text == "" {
			return nil
		}
		m.emit(inbox.Event{Kind: m.composeKind, Text: text, File: m.anchorFile, Lines: m.anchorLines, Comment: m.composeRef})
	case tea.KeyCtrlX:
		m.anchorFile, m.anchorLines, m.composeRef = "", "", 0
	case tea.KeyLeft:
		m.inputPos = max(m.inputPos-1, 0)
	case tea.KeyRight:
		m.inputPos = min(m.inputPos+1, len(m.input))
	case tea.KeyCtrlA, tea.KeyHome:
		m.inputPos = 0
	case tea.KeyCtrlE, tea.KeyEnd:
		m.inputPos = len(m.input)
	case tea.KeyBackspace:
		if m.inputPos > 0 {
			m.input = append(m.input[:m.inputPos-1], m.input[m.inputPos:]...)
			m.inputPos--
		}
	case tea.KeyCtrlW:
		from := m.inputPos
		for from > 0 && m.input[from-1] == ' ' {
			from--
		}
		for from > 0 && m.input[from-1] != ' ' {
			from--
		}
		m.input = append(m.input[:from], m.input[m.inputPos:]...)
		m.inputPos = from
	case tea.KeyCtrlU:
		m.input, m.inputPos = nil, 0
	case tea.KeyRunes, tea.KeySpace:
		m.insert(msg.Runes)
	}
	return nil
}

func (m *model) insert(rs []rune) {
	m.inputPos = min(m.inputPos, len(m.input))
	tail := append([]rune{}, m.input[m.inputPos:]...)
	m.input = append(append(m.input[:m.inputPos], rs...), tail...)
	m.inputPos += len(rs)
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
