package view

import (
	"cmp"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/pltanton/guided-review/internal/state"
)

func modalBox(w, h int, title, hint string, body []string) []string {
	w, h = max(w, 8), max(h, 3)
	frame := faintTone.fg()
	head := frame.Render("┌ ") + boldStyle.Render(ansi.Truncate(title, w-6, "…")) + " "
	head += frame.Render(strings.Repeat("─", max(w-ansi.StringWidth(head)-1, 0)) + "┐")
	hint = ansi.Truncate(hint, w-4, "…")
	foot := frame.Render("└" + strings.Repeat("─", max(w-ansi.StringWidth(hint)-3, 0)))
	foot += " " + dimStyle.Render(hint) + frame.Render("┘")
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

func (m *model) modal(w int) (modalContent, bool) {
	if m.chapterOpen != "" {
		return m.chapterModal(w), true
	}
	if m.help {
		lines := m.helpLines(w)
		hint := "? all keys · any key closes"
		if m.helpAll {
			hint = "j/k scroll · any key closes · remap in " + configHint
		}
		return modalContent{"keys", hint, lines[min(m.helpTop, len(lines)):]}, true
	}
	return modalContent{}, false
}

func (m *model) drawModal(out []string) {
	if len(out) < 4 {
		return
	}
	w, h := max(m.width-2, 8), len(out)-2
	if c, ok := m.modal(w - 4); ok {
		overlayAt(out, modalBox(w, h, c.title, c.hint, c.body), 1, 1)
	}
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
