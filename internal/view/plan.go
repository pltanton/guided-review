package view

import (
	"cmp"
	"fmt"
	"slices"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/pltanton/guided-review/internal/state"
)

const sideContext = 3

type planItem struct {
	st      state.Step
	chapter string
	intro   string
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
			head.intro = cmp.Or(head.intro, st.Intro)
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

func (m *model) focusPlanPanel() {
	if m.review == nil || len(m.review.Steps) == 0 {
		return
	}
	m.focusPlan, m.focusFiles = true, false
	m.planCursor = m.planAnchor(m.planItems())
}

// openChapter opens the plan on the current step's chapter, unfolded so its intro shows.
func (m *model) openChapter() {
	if m.step == nil || m.review == nil {
		return
	}
	m.focusPlanPanel()
	ch := m.step.Chapter
	if ch == "" {
		return
	}
	m.setChapterOpen(ch, true)
	i := slices.IndexFunc(m.planItems(), func(it planItem) bool { return it.head && it.chapter == ch })
	m.planCursor = max(i, 0)
}

func (m *model) introOnce() {
	st := m.step
	if st == nil || st.Chapter == "" || m.introShown[st.Chapter] {
		return
	}
	if !slices.ContainsFunc(m.review.Steps, func(s state.Step) bool {
		return s.Chapter == st.Chapter && s.Intro != ""
	}) {
		return
	}
	if m.introShown == nil {
		m.introShown = map[string]bool{}
	}
	m.introShown[st.Chapter] = true
	m.openChapter()
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
	case "act", "message", "open":
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
	case "back", "steps", "plan":
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
	cw := m.chatWidth()
	if cw == 0 || x < m.width-cw || m.step == nil {
		return false
	}
	side := m.sideFiles(m.height-len(m.bottomLines()), cw)
	if y < 0 || y >= len(side) || !side[y].files {
		return false
	}
	m.fileTop += d
	if m.focusFiles {
		m.fileCursor = max(0, min(m.fileCursor+d, len(m.stepFiles())-1))
	}
	return true
}
