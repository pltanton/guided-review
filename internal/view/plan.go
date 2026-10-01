package view

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/pltanton/guided-review/internal/state"
)

const sideContext = 3

type planItem struct {
	st      state.Step
	chapter string
	head    bool
	open    bool
	current bool
	done    int
	total   int
	hot     int
	extra   bool
}

func (m *model) planItems() []planItem {
	steps := m.review.Steps
	shown := ""
	if m.step != nil {
		shown = m.step.ID
	}
	var out []planItem
	for i := 0; i < len(steps); {
		ch := steps[i].Chapter
		j := i + 1
		for ch != "" && j < len(steps) && steps[j].Chapter == ch {
			j++
		}
		if ch == "" {
			out = append(out, planItem{st: steps[i]})
			i = j
			continue
		}
		head := planItem{chapter: ch, head: true, total: j - i}
		here := false
		for _, st := range steps[i:j] {
			if st.Status != state.StatusPending {
				head.done++
			}
			if st.Status == state.StatusPending || st.Status == state.StatusStale {
				head.hot += len(st.Hotspots)
			}
			head.current = head.current || st.ID == m.review.Current
			here = here || st.ID == shown
		}
		open, set := m.planOpen[ch]
		head.open = open || !set && (head.current || here)
		out = append(out, head)
		if head.open {
			for _, st := range steps[i:j] {
				out = append(out, planItem{st: st, chapter: ch})
			}
		}
		i = j
	}
	for _, st := range m.extraSteps() {
		out = append(out, planItem{st: st, extra: true})
	}
	return out
}

func (m *model) planAnchor(items []planItem) int {
	id := m.review.Current
	if m.step != nil {
		id = m.step.ID
	}
	ch := ""
	if st := m.stepByID(id); st != nil {
		ch = st.Chapter
	}
	head := 0
	for i, it := range items {
		switch {
		case !it.head && it.st.ID == id:
			return i
		case it.head && it.chapter == ch:
			head = i
		}
	}
	return head
}

func (m *model) setChapterOpen(ch string, open bool) {
	if m.planOpen == nil {
		m.planOpen = map[string]bool{}
	}
	m.planOpen[ch] = open
}

func (m *model) toggleChapter(ch string) {
	for _, it := range m.planItems() {
		if it.head && it.chapter == ch {
			m.setChapterOpen(ch, !it.open)
			return
		}
	}
}

func (m *model) focusPlanPanel() {
	if m.review == nil || len(m.review.Steps) == 0 {
		return
	}
	m.showPlan, m.focusPlan, m.focusFiles = true, true, false
	m.planCursor = m.planAnchor(m.planItems())
	m.relist()
}

func (m *model) handlePlanKey(msg tea.KeyMsg) tea.Cmd {
	items := m.planItems()
	m.planCursor = max(0, min(m.planCursor, len(items)-1))
	var it planItem
	if len(items) > 0 {
		it = items[m.planCursor]
	}
	switch m.keys().name(msg.String()) {
	case "quit":
		return tea.Quit
	case "down":
		m.planCursor++
	case "up":
		m.planCursor--
	case "top":
		m.planCursor = 0
	case "bottom":
		m.planCursor = len(items) - 1
	case "message", "open":
		if it.head {
			m.setChapterOpen(it.chapter, !it.open)
			break
		}
		m.focusPlan = false
		return m.showStep(it.st.ID)
	case "scroll-left":
		if it.chapter == "" {
			break
		}
		m.setChapterOpen(it.chapter, false)
		for i := m.planCursor; i >= 0; i-- {
			if items[i].head && items[i].chapter == it.chapter {
				m.planCursor = i
				break
			}
		}
	case "scroll-right":
		if it.head {
			m.setChapterOpen(it.chapter, true)
		}
	case "files":
		m.focusPlan = false
		m.focusFilesPanel()
	case "plan":
		m.focusPlan, m.showPlan = false, false
		m.relist()
	case "back", "steps":
		m.focusPlan = false
	}
	m.planCursor = max(0, min(m.planCursor, len(m.planItems())-1))
	return nil
}

func (m *model) panelHint() string {
	k := m.keys()
	switch {
	case m.focusPlan:
		return fmt.Sprintf("%s/%s move · %s open · %s/%s fold · %s files · %s back",
			k.key("down"), k.key("up"), k.key("open"), k.key("scroll-left"),
			k.key("scroll-right"), k.key("files"), k.key("back"))
	case m.focusFiles:
		return fmt.Sprintf("%s/%s move · %s open · %s plan · %s back",
			k.key("down"), k.key("up"), k.key("open"), k.key("steps"), k.key("back"))
	}
	return ""
}

func follow(top, at, h, n int) int {
	ctx := max(min(sideContext, (h-1)/2), 0)
	top = max(min(top, at-ctx), at+ctx-h+1)
	return max(0, min(top, n-h))
}

func scrollWindow(top *int, last *string, key string, force bool, at, h, n int) int {
	if force || *last != key {
		*top, *last = follow(*top, at, h, n), key
	}
	*top = max(0, min(*top, n-h))
	return *top
}

func (m *model) sideWheel(x, y, d int) bool {
	pw := m.planWidth()
	if x >= pw || m.step == nil {
		return false
	}
	side := m.sidebar(max(m.height-len(m.bottomLines()), 1), pw)
	if y < 0 || y >= len(side) {
		return false
	}
	switch side[y].zone {
	case zonePlan:
		m.planTop += d
		if m.focusPlan {
			m.planCursor += d
		}
	case zoneFiles:
		m.fileTop += d
		if m.focusFiles {
			m.fileCursor = max(0, min(m.fileCursor+d, len(m.stepFiles())-1))
		}
	default:
		return false
	}
	return true
}
