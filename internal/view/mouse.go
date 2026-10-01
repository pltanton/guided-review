package view

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/pltanton/guided-review/internal/inbox"
)

const wheelStep = 3

func (m *model) handleMouse(msg tea.MouseMsg) tea.Cmd {
	if m.preview == "" && m.popup == nil && m.chatMouse(msg) {
		return nil
	}
	switch {
	case m.preview != "" && msg.Button == tea.MouseButtonWheelUp:
		m.previewTop = max(m.previewTop-wheelStep, 0)
	case m.preview != "" && msg.Button == tea.MouseButtonWheelDown:
		m.previewTop += wheelStep
	case m.preview != "":
	case m.popup != nil && msg.Button == tea.MouseButtonWheelUp:
		m.popup.top = max(m.popup.top-wheelStep, 0)
		m.popup.sel = max(m.popup.sel-1, 0)
	case m.popup != nil && msg.Button == tea.MouseButtonWheelDown:
		m.popup.top = min(m.popup.top+wheelStep, max(len(m.popup.lines)-1, 0))
		m.popup.sel = min(m.popup.sel+1, max(len(m.popup.items)-1, 0))
	case msg.Button == tea.MouseButtonWheelUp && m.overChat(msg.X, msg.Y):
		m.chatTop += wheelStep
	case msg.Button == tea.MouseButtonWheelDown && m.overChat(msg.X, msg.Y):
		m.chatTop = max(m.chatTop-wheelStep, 0)
	case msg.Button == tea.MouseButtonWheelUp:
		m.scroll(-wheelStep)
	case msg.Button == tea.MouseButtonWheelDown:
		m.scroll(wheelStep)
	case msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft:
		if m.resizing = m.separatorAt(msg.X, msg.Y); m.resizing != "" {
			return nil
		}
		if i := m.optionAt(msg.X, msg.Y); i >= 0 {
			m.answer(i)
			return nil
		}
		if m.onChatInput(msg.X, msg.Y) {
			if !m.composing {
				m.startCompose(inbox.KindMessage)
			}
			return nil
		}
		if cw := m.chatWidth(); cw > 0 && msg.X > m.width-cw {
			return nil
		}
		if msg.Y == m.height-1 && !m.composing {
			_, spans := m.footer()
			for _, sp := range spans {
				if msg.X >= sp.from && msg.X < sp.to {
					return sp.b.press(m)
				}
			}
			return nil
		}
		if pw := m.planWidth(); msg.X < pw {
			side := m.sidebar(max(m.height-len(m.bottomLines()), 1), pw)
			if msg.Y < len(side) {
				switch e := side[msg.Y]; {
				case e.step != "":
					return m.showStep(e.step)
				case e.file != "":
					m.jumpToFile(e.file)
				}
			}
			return nil
		}
		if i, sub, ok := m.rowAt(msg.Y); ok {
			m.cursor, m.anchor, m.dragging, m.visual = i, i, true, false
			m.clamp()
			if !m.useSplit() {
				start := m.hscroll
				if !m.nowrap {
					start = sub * codeAvail(m.mainWidth())
				}
				m.col = start + max(msg.X-m.planWidth()-codePrefix, 0)
			}
			if it := m.lines[i]; it.Kind == RowFold || it.GapTo > 0 {
				m.toggleFold()
			}
		}
	case msg.Action == tea.MouseActionMotion && m.resizing != "":
		switch m.resizing {
		case "plan":
			m.planW = msg.X + 1
		case "chat":
			m.sideW = m.width - msg.X
		}
		m.relist()
	case msg.Action == tea.MouseActionMotion && m.dragging:
		if i, _, ok := m.rowAt(msg.Y); ok {
			m.cursor = i
			m.visual = m.cursor != m.anchor
			m.clamp()
		}
	case msg.Action == tea.MouseActionRelease:
		m.dragging, m.resizing = false, ""
	}
	return nil
}

func (m *model) scroll(d int) {
	body := m.bodyHeight()
	m.offset = max(0, min(m.offset+d, len(m.lines)-body))
	m.cursor = max(m.offset, min(m.cursor, m.lastShown()))
	m.cursor = max(0, min(m.cursor, len(m.lines)-1))
}

func (m *model) rowAt(y int) (i, sub int, ok bool) {
	hdr := len(m.header())
	if y < hdr || y >= hdr+m.bodyHeight() {
		return 0, 0, false
	}
	sub = y - hdr
	for i = m.offset; i < len(m.lines); i++ {
		h := m.lineHeight(i)
		if sub < h {
			return i, sub, true
		}
		sub -= h
	}
	return 0, 0, false
}

func (m *model) overChat(x, y int) bool {
	if cw := m.chatWidth(); cw > 0 {
		return x >= m.width-cw
	}
	return y >= m.height-len(m.bottomLines())
}

func (m *model) separatorAt(x, y int) string {
	bottom := m.bottomLines()
	top := m.height - len(bottom)
	switch {
	case m.step == nil || m.preview != "":
		return ""
	case y < top && x == m.planWidth()-1:
		return "plan"
	case y < top && m.chatWidth() > 0 && x == m.width-m.chatWidth():
		return "chat"
	}
	return ""
}

func (m *model) onChatInput(x, y int) bool {
	bodyH := m.height - len(m.bottomLines())
	_, top, _, pills := m.pillsAt()
	switch {
	case pills && m.chatWidth() > 0:
		return x > m.width-m.chatWidth() && y >= top-1 && y < bodyH
	case pills:
		return y >= top && y < m.height-1
	case m.step == nil:
		return y == m.height-2
	case m.chatWidth() > 0:
		return x > m.width-m.chatWidth() && y >= bodyH-2 && y < bodyH
	}
	return false
}

func (m *model) inputRowX() int {
	switch {
	case m.step == nil:
		return max((m.width-min(max(m.width-2, 20), intakeWidth))/2, 0)
	case m.chatWidth() > 0:
		return m.width - m.chatWidth() + 2
	}
	return 0
}
