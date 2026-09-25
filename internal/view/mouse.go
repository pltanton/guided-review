package view

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/aplotnikov/guided-review/internal/inbox"
)

const wheelStep = 3

func (m *model) handleMouse(msg tea.MouseMsg) tea.Cmd {
	switch {
	case msg.Button == tea.MouseButtonWheelUp:
		m.scroll(-wheelStep)
	case msg.Button == tea.MouseButtonWheelDown:
		m.scroll(wheelStep)
	case msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft:
		if msg.X < m.planWidth() {
			if id := m.planStepAt(msg.Y); id != "" {
				m.emit(inbox.Event{Kind: inbox.KindGoto, Step: id})
			}
			return nil
		}
		if i, ok := m.rowAt(msg.Y); ok {
			m.cursor, m.anchor, m.dragging, m.visual = i, i, true, false
			m.clamp()
		}
	case msg.Action == tea.MouseActionMotion && m.dragging:
		if i, ok := m.rowAt(msg.Y); ok {
			m.cursor = i
			m.visual = m.cursor != m.anchor
			m.clamp()
		}
	case msg.Action == tea.MouseActionRelease:
		m.dragging = false
	}
	return nil
}

func (m *model) scroll(d int) {
	body := m.bodyHeight()
	m.offset = max(0, min(m.offset+d, len(m.list)-body))
	m.cursor = max(m.offset, min(m.cursor, m.offset+body-1))
	m.cursor = max(0, min(m.cursor, len(m.list)-1))
}

func (m *model) rowAt(y int) (int, bool) {
	hdr := len(m.header())
	if y < hdr || y >= hdr+m.bodyHeight() {
		return 0, false
	}
	i := m.offset + y - hdr
	return i, i < len(m.list)
}

func (m *model) planStepAt(y int) string {
	i := m.planOffset() + y - 1
	if y < 1 || i >= len(m.review.Steps) {
		return ""
	}
	return m.review.Steps[i].ID
}
