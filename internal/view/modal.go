package view

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/charmbracelet/x/ansi"

	"github.com/pltanton/guided-review/internal/config"
	"github.com/pltanton/guided-review/internal/state"
)

func modalBox(w, h int, title, hint string, body []string) []string {
	w, h = max(w, 8), max(h, 3)
	frame := faintTone.fg()
	head := frame.Render("┌ ") + boldStyle.Render(ansi.Truncate(title, w-6, "…")) + " "
	head += frame.Render(strings.Repeat("─", max(w-ansi.StringWidth(head)-1, 0)) + "┐")
	foot := frame.Render("└" + strings.Repeat("─", w-2) + "┘")
	if hint = ansi.Truncate(hint, w-4, "…"); hint != "" {
		foot = frame.Render("└" + strings.Repeat("─", max(w-ansi.StringWidth(hint)-3, 0)))
		foot += " " + dimStyle.Render(hint) + frame.Render("┘")
	}
	out := []string{head}
	for i := range h - 2 {
		line := ""
		if i < len(body) {
			line = body[i]
		}
		out = append(out, frame.Render("│ ")+fit(line, w-4)+frame.Render(" │"))
	}
	return append(out, foot)
}

func overlayAt(base, box []string, top, x int) {
	for i, b := range box {
		j := top + i
		if j < 0 || j >= len(base) {
			continue
		}
		rest := ansi.TruncateLeft(base[j], x+ansi.StringWidth(b), "")
		base[j] = fit(base[j], x) + b + rest
	}
}

type modalContent struct {
	title, hint string
	body        []string
}

func (m *model) modal(w, h int) (modalContent, bool) {
	if m.help {
		lines := m.helpLines(w)
		hint := "? all keys · any key closes"
		if m.helpAll {
			hint = "j/k scroll · any key closes · remap in " + configHint
		}
		m.helpTop = max(0, min(m.helpTop, len(lines)-h))
		return modalContent{"keys", hint, lines[m.helpTop:]}, true
	}
	switch {
	case m.chapterOpen != "":
		return m.chapterModal(w), true
	case m.gateOpen:
		return m.gateModal(w), true
	case m.finishCard:
		return m.finishCardModal(w), true
	case m.pub != nil:
		return m.publishModal(w), true
	case m.threadCard != "":
		return m.threadCardModal(w), true
	case m.newsOpen:
		return m.newsModal(w, h), true
	case len(m.staleSteps) > 0:
		return m.staleModal(w), true
	case m.focusPlan:
		return m.planModal(w, h), true
	case m.focusFiles && m.chatWidth() == 0:
		return m.filesModal(w, h), true
	case m.popup != nil && m.popup.kind != "hover":
		return modalContent{m.popup.title, m.popupHint(), m.popupBody(w, h)}, true
	}
	return modalContent{}, false
}

const (
	cardWidth     = 100
	wideCardWidth = 154
)

func (m *model) fullModal() bool {
	return !m.help && m.popup != nil && m.popup.kind != "hover" && m.popup.kind != "detail" &&
		m.chapterOpen == "" && !m.gateOpen && !m.finishCard && m.pub == nil &&
		m.threadCard == "" && !m.newsOpen &&
		len(m.staleSteps) == 0 &&
		!m.focusPlan
}

func (m *model) drawModal(out []string) {
	if len(out) < 4 {
		return
	}
	room := len(out) - 2
	if m.themeWas != "" {
		c := m.themeModal()
		w, h := min(max(m.width-4, 8), themeCardWidth), min(len(c.body)+2, room)
		m.modalY = 1
		overlayAt(out, modalBox(w, h, c.title, c.hint, c.body), m.modalY, m.width-w-1)
		return
	}
	w, h := max(m.width, 8), room
	if !m.fullModal() {
		w = min(max(m.width-4, 8), cardWidth)
		if m.help && m.helpAll {
			w = min(max(m.width-4, 8), wideCardWidth)
		}
	}
	c, ok := m.modal(w-4, h-2)
	if !ok {
		if m.popup != nil && m.popup.kind == "hover" {
			m.drawHover(out)
		}
		m.drawPalette(out)
		return
	}
	if !m.fullModal() {
		h = min(len(c.body)+2, room)
	}
	for i := 1; i < len(out)-1; i++ {
		out[i] = faintTone.fg().Render(ansi.Strip(out[i]))
	}
	m.modalY = 1 + (room-h)/2
	overlayAt(out, modalBox(w, h, c.title, c.hint, c.body), m.modalY, (m.width-w)/2)
}

const paletteRows = 8

func (m *model) drawPalette(out []string) {
	items := m.paletteItems()
	if len(items) == 0 {
		return
	}
	sel := min(m.paletteSel, len(items)-1)
	start := max(0, min(sel-paletteRows/2, len(items)-paletteRows))
	w := min(m.width, 100)
	var body []string
	for i := start; i < len(items) && len(body) < paletteRows; i++ {
		a := items[i]
		key := ""
		if len(a.Keys) > 0 {
			key = keyLabel(a.Keys[0])
		}
		row := fmt.Sprintf("%-16s %s", a.Name, dimStyle.Render(a.Desc))
		row = fit(row, max(w-4-ansi.StringWidth(key)-1, 1)) + " " + cursorStyle.Render(key)
		if i == sel {
			row = paint(fit(row, w-4), cursorTone)
		}
		body = append(body, row)
	}
	box := modalBox(w, len(body)+2, "actions", "↑/↓ choose · enter run · esc", body)
	overlayAt(out, box, max(len(out)-1-len(box), 0), 0)
}

func (m *model) drawHover(out []string) {
	mw := m.mainWidth()
	w := min(max(mw-4, 20), detailWidth)
	lines := m.popup.lines[min(m.popup.top, max(len(m.popup.lines)-1, 0)):]
	h := min(len(lines)+2, max((len(out)-2)/2, 3))
	y := m.cursorY() + 1
	if y+h > len(out)-1 {
		y = max(m.cursorY()-h, 1)
	}
	overlayAt(out, modalBox(w, h, "hover", "esc close", lines), y, 2)
}

func (m *model) cursorY() int {
	y := len(m.header())
	for i := m.offset; i < m.cursor && i < len(m.lines); i++ {
		y += m.lineHeight(i)
	}
	return y
}

func (m *model) chapterIntro(st *state.Step) string {
	if st.Chapter == "" || m.review == nil {
		return ""
	}
	for _, s := range m.review.Steps {
		if s.Chapter == st.Chapter && s.Intro != "" {
			return s.Intro
		}
	}
	return ""
}

func (m *model) introOnce() {
	st := m.step
	if st == nil || st.Chapter == "" || m.introShown[st.Chapter] || m.chapterIntro(st) == "" {
		return
	}
	if m.introShown == nil {
		m.introShown = map[string]bool{}
	}
	m.introShown[st.Chapter], m.chapterOpen = true, st.Chapter
}

func (m *model) openChapter() {
	if m.step == nil || m.chapterIntro(m.step) == "" {
		m.status = "this step has no chapter intro"
		return
	}
	m.chapterOpen = m.step.Chapter
}

func (m *model) chapterModal(w int) modalContent {
	var intro string
	var steps []string
	for _, st := range m.review.Steps {
		if st.Chapter != m.chapterOpen {
			continue
		}
		intro = cmp.Or(intro, st.Intro)
		glyph, style := st.Status.Glyph(), dimStyle
		switch {
		case st.ID == m.review.Current:
			glyph, style = "▶", boldStyle
		case st.Status == state.StatusPending:
			glyph, style = "○", textTone.fg()
		}
		steps = append(steps, style.Render(glyph+" "+st.ID+" "+st.Title))
	}
	body := append(markdownLines(intro, min(w, detailWidth)), "")
	return modalContent{m.chapterOpen, "enter close · I reopens", append(body, steps...)}
}

func (m *model) planModal(w, h int) modalContent {
	items := m.planItems()
	m.planCursor = max(0, min(m.planCursor, len(items)-1))
	row, plain, selected := sideRows(w + 2)
	entries := m.planEntries(items, row, plain, selected)
	key := fmt.Sprint(h, " ", m.planCursor)
	top := scrollWindow(&m.planTop, &m.planFollow, key, true, m.planCursor, h, len(entries))
	body := entries[top:]
	reviewed := 0
	for _, st := range m.review.Steps {
		if st.Status != state.StatusPending {
			reviewed++
		}
	}
	title := fmt.Sprintf("plan · %d/%d reviewed", reviewed, len(m.review.Steps))
	if m.review.Round > 1 {
		title += fmt.Sprintf(" · round %d", m.review.Round)
	}
	return modalContent{title, "enter show · h/l fold · esc close", body}
}

func (m *model) filesModal(w, h int) modalContent {
	row, plain, selected := sideRows(w + 2)
	entries, at := m.fileEntries(row, plain, selected)
	key := fmt.Sprint(h, " ", at)
	top := scrollWindow(&m.fileTop, &m.fileFollow, key, true, at, h, len(entries))
	var body []string
	for _, e := range entries[top:] {
		body = append(body, e.text)
	}
	return modalContent{"files · " + m.step.ID, "enter jump · esc close", body}
}

func (m *model) modalMouse(msg tea.MouseMsg) tea.Cmd {
	wheel := map[tea.MouseButton]int{tea.MouseButtonWheelUp: -1, tea.MouseButtonWheelDown: 1}
	press := msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft
	r := msg.Y - m.modalY - 1
	switch {
	case m.focusPlan && wheel[msg.Button] != 0:
		m.planCursor = max(0, min(m.planCursor+wheel[msg.Button], len(m.planItems())-1))
	case m.focusPlan && press && r >= 0 && m.planTop+r < len(m.planItems()):
		m.planCursor = m.planTop + r
		return m.handlePlanKey(tea.KeyMsg{Type: tea.KeyEnter})
	case m.focusFiles && wheel[msg.Button] != 0:
		m.fileCursor = max(0, min(m.fileCursor+wheel[msg.Button], len(m.stepFiles())-1))
	case m.focusFiles && press && r >= 0:
		row, plain, selected := sideRows(m.width)
		entries, _ := m.fileEntries(row, plain, selected)
		if i := m.fileTop + r; i < len(entries) && entries[i].file != "" {
			m.jumpToFile(entries[i].file)
			m.focusFiles = false
		}
	}
	return nil
}

func (m *model) gateModal(w int) modalContent {
	var body []string
	item := func(i int, text string) { body = append(body, cardItem(i, m.gateSel, text, w)) }
	if len(m.step.Hotspots) > 0 {
		body = append(body, dimStyle.Render("risks — enter checks one off once you are sure"), "")
	}
	for i, h := range m.step.Hotspots {
		mark := hotStyle.Render("⚑ ")
		if h.Checked {
			mark = addStyle.Render("✓ ")
		}
		for k, l := range strings.Split(ansi.Wrap(h.Q, max(w-4, 10), ""), "\n") {
			if k > 0 {
				mark = "  "
			}
			if k == 0 {
				item(i, mark+l)
			} else {
				body = append(body, "   "+l)
			}
		}
	}
	if n := m.unseen(); n > 0 {
		body = append(body, "", hotStyle.Render(fmt.Sprintf("%d changed lines not seen yet", n)))
	}
	if waiting := m.threadsWaiting(); waiting != "" {
		body = append(body, "", waiting)
	}
	body = append(body, "")
	item(len(m.step.Hotspots), boldStyle.Render("move on to the next step"))
	return modalContent{"before " + m.step.ID + " is done", "enter · esc back to the step", body}
}

func (m *model) blocked() bool {
	return m.help || m.chapterOpen != "" || m.gateOpen || m.finishCard || m.pub != nil ||
		m.threadCard != "" || m.themeWas != "" || m.newsOpen ||
		len(m.staleSteps) > 0 || m.focusPlan ||
		m.popup != nil && m.popup.kind != "hover"
}

var staleItems = []string{"continue with the other steps", "finish now"}

func (m *model) staleModal(w int) modalContent {
	body := []string{
		hotStyle.Render("a blocker: these steps may change once it is fixed"),
		"   " + strings.Join(m.staleSteps, " "),
		dimStyle.Render("they stay in the plan marked ~ and go to the next round if you finish now"),
		"",
	}
	for i, it := range staleItems {
		body = append(body, cardItem(i, m.staleSel, it, w))
	}
	return modalContent{"blocker", "enter · esc continue", body}
}

func (m *model) handleStaleKey(msg tea.KeyMsg) tea.Cmd {
	switch cardNav(msg.String(), len(staleItems), &m.staleSel) {
	case "esc":
		m.staleSteps = nil
	case "enter":
		finish := m.staleSel == 1
		m.staleSteps = nil
		if finish {
			m.openFinish()
		}
	}
	return nil
}

func cardItem(i, sel int, text string, w int) string {
	text = keyStyle.Render(fmt.Sprintf(" %d ", i+1)) + " " + text
	if i == sel {
		return paint(fit(accentTone.fg().Render("▌")+text, w), cursorTone)
	}
	return " " + text
}

func digitPick(k string, n int) (int, bool) {
	if len(k) != 1 || k < "1" || k > "9" || int(k[0]-'1') >= n {
		return 0, false
	}
	return int(k[0] - '1'), true
}

// cardNav moves sel over n items; it reports "enter" (a digit picks too) or "esc".
func cardNav(k string, n int, sel *int) string {
	if i, ok := digitPick(k, n); ok {
		*sel = i
		return "enter"
	}
	switch k {
	case "j", "down":
		*sel = min(*sel+1, max(n-1, 0))
	case "k", "up":
		*sel = max(*sel-1, 0)
	case "esc", "q", "enter":
		return k
	}
	return ""
}

const themeCardWidth = 34

func (m *model) openThemes() {
	m.themeWas, m.themeSel = themeName, max(slices.Index(Themes(), themeName), 0)
}

func (m *model) themeModal() modalContent {
	var body []string
	for i, name := range Themes() {
		if name == m.themeWas {
			name += dimStyle.Render("  (was)")
		}
		body = append(body, cardItem(i, m.themeSel, name, themeCardWidth-4))
	}
	return modalContent{"theme", "j/k try · enter save · esc", body}
}

func (m *model) handleThemeKey(msg tea.KeyMsg) tea.Cmd {
	names := Themes()
	k := msg.String()
	if i, ok := digitPick(k, len(names)); ok {
		m.themeSel, k = i, ""
	}
	switch cardNav(k, len(names), &m.themeSel) {
	case "esc":
		m.setTheme(m.themeWas)
		m.themeWas = ""
	case "enter":
		m.themeWas = ""
		m.saveTheme(names[m.themeSel])
	default:
		m.setTheme(names[m.themeSel])
	}
	return nil
}

func (m *model) setTheme(name string) {
	if name == themeName {
		return
	}
	if err := applyTheme(name); err != nil {
		m.err = err
		return
	}
	if m.src != nil {
		m.src = newGitSource(m.ctx, m.repo, m.algo, m.src.base, m.src.head)
		m.rebuild(false)
	}
}

func (m *model) saveTheme(name string) {
	path, err := config.UserPath()
	if err == nil {
		err = config.SaveTheme(path, name)
	}
	if err != nil {
		m.err = fmt.Errorf("theme %s is on for now, saving it failed: %w", name, err)
		return
	}
	m.status = "theme " + name + " saved to " + path
}
