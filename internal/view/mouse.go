package view

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/pltanton/guided-review/internal/inbox"
)

const wheelStep = 3

func (m *model) handleMouse(msg tea.MouseMsg) tea.Cmd {
	switch {
	case m.newsOpen && msg.Button == tea.MouseButtonWheelDown:
		m.newsTop += wheelStep
		return nil
	case m.newsOpen && msg.Button == tea.MouseButtonWheelUp:
		m.newsTop = max(m.newsTop-wheelStep, 0)
		return nil
	case m.newsOpen:
		return nil
	case m.gateOpen || m.finishCard || m.pub != nil || m.threadCard != "" ||
		m.themeWas != "" || len(m.staleSteps) > 0:
		return nil
	case m.chapterOpen != "":
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			m.chapterOpen = ""
		}
		return nil
	case m.focusPlan || m.focusFiles && m.chatWidth() == 0:
		return m.modalMouse(msg)
	case m.help && msg.Button == tea.MouseButtonWheelUp:
		m.helpTop = max(m.helpTop-wheelStep, 0)
		return nil
	case m.help && msg.Button == tea.MouseButtonWheelDown:
		m.helpTop += wheelStep
		return nil
	case m.help:
		return nil
	case m.threads && msg.Button == tea.MouseButtonWheelUp:
		m.threadTop = max(m.threadTop-wheelStep, 0)
		return nil
	case m.threads && msg.Button == tea.MouseButtonWheelDown:
		m.threadTop += wheelStep
		return nil
	case m.threads:
		return nil
	}
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
	case m.popup != nil && m.popup.kind != "hover":
	case msg.Button == tea.MouseButtonWheelUp && m.sideWheel(msg.X, msg.Y, -wheelStep):
	case msg.Button == tea.MouseButtonWheelDown && m.sideWheel(msg.X, msg.Y, wheelStep):
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
			side := m.sideFiles(m.height-len(m.bottomLines()), cw)
			if msg.Y < len(side) && side[msg.Y].file != "" {
				m.jumpToFile(side[msg.Y].file)
			}
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
		if i, sub, ok := m.rowAt(msg.Y); ok {
			m.cursor, m.anchor, m.dragging, m.visual = i, i, true, false
			m.clamp()
			if !m.useSplit() {
				start := m.hscroll
				if !m.nowrap {
					start = sub * codeAvail(m.mainWidth())
				}
				m.col = start + max(msg.X-codePrefix, 0)
			}
			if it := m.lines[i]; it.Kind == RowFold || it.GapTo > 0 {
				m.toggleFold()
			}
		}
	case msg.Action == tea.MouseActionMotion && m.resizing != "":
		if m.resizing == "chat" {
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
		return y == m.height-3
	case m.chatWidth() > 0:
		return x > m.width-m.chatWidth() && y >= bodyH-2 && y < bodyH
	}
	return false
}

func (m *model) inputRowX() int {
	switch {
	case m.step == nil:
		x, _, _ := m.cardFrame(intakeWidth)
		return x
	case m.chatWidth() > 0:
		return m.width - m.chatWidth() + 2
	}
	return 0
}
