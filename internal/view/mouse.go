package view

import (
	tea "github.com/charmbracelet/bubbletea"
)

const wheelStep = 3

func (m *model) handleMouse(msg tea.MouseMsg) tea.Cmd {
	switch {
	case msg.Button == tea.MouseButtonWheelUp:
		m.scroll(-wheelStep)
	case msg.Button == tea.MouseButtonWheelDown:
		m.scroll(wheelStep)
	case msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft:
		if msg.Y == m.height-1 && !m.composing {
			_, spans := m.footer()
			for _, sp := range spans {
				if msg.X >= sp.from && msg.X < sp.to {
					sp.b.press(m)
				}
			}
			return nil
		}
		if pw := m.planWidth(); msg.X < pw {
			side := m.sidebar(max(m.height-len(m.bottomLines()), 1), pw)
			if msg.Y < len(side) {
				switch e := side[msg.Y]; {
				case e.step != "":
					m.showStep(e.step)
				case e.file != "":
					m.jumpToFile(e.file)
				}
			}
			return nil
		}
		if i, ok := m.rowAt(msg.Y); ok {
			m.cursor, m.anchor, m.dragging, m.visual = i, i, true, false
			m.clamp()
			if i < len(m.disp) && !m.useSplit() && m.disp[i].Kind == RowFold {
				m.toggleFold()
			}
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
