package view

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/aplotnikov/guided-review/internal/inbox"
	"github.com/aplotnikov/guided-review/internal/state"
)

var (
	addStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	delStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	hotStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true)
	dimStyle    = lipgloss.NewStyle().Faint(true)
	boldStyle   = lipgloss.NewStyle().Bold(true)
	fileStyle   = lipgloss.NewStyle().Bold(true).Underline(true)
	cursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true)
	selectStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("5")).Bold(true)
	agentStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true)

	noteStyles = map[string]lipgloss.Style{
		"note":    lipgloss.NewStyle().Foreground(lipgloss.Color("6")),
		"spec":    lipgloss.NewStyle().Foreground(lipgloss.Color("1")),
		"hotspot": hotStyle,
		"comment": lipgloss.NewStyle().Foreground(lipgloss.Color("5")),
		"mr":      lipgloss.NewStyle().Foreground(lipgloss.Color("4")),
	}
	noteIcons = map[string]string{"note": "●", "spec": "⚠", "hotspot": "⚑", "comment": "✎", "mr": "💬"}

	statusGlyph = map[state.StepStatus]string{
		state.StatusPending: "·", state.StatusDone: "✓", state.StatusSkipped: "↷", state.StatusStale: "~",
	}
)

const hints = "c message  ? explain  > next  S skip  v select  n note  s split  p plan  e editor  q quit"

func (m *model) planWidth() int {
	if !m.showPlan || m.review == nil || len(m.review.Steps) == 0 || m.width < minPlanWidth {
		return 0
	}
	return min(maxPlanWidth, m.width/4)
}

func (m *model) mainWidth() int {
	return m.width - m.planWidth()
}

func (m *model) bodyHeight() int {
	return max(m.height-len(m.bottomLines())-len(m.header()), 1)
}

func (m *model) header() []string {
	if m.step == nil {
		return nil
	}
	st := m.step
	title := fmt.Sprintf("%s %d/%d %s · %s", st.ID, m.review.StepIndex(st.ID)+1, len(m.review.Steps), st.Kind, st.Title)
	if st.Status != state.StatusPending {
		title += " [" + string(st.Status) + "]"
	}
	if m.review.Round > 1 {
		title += fmt.Sprintf("  round %d", m.review.Round)
	}
	lines := []string{boldStyle.Render(title)}
	if st.Note != "" {
		lines = append(lines, dimStyle.Render(st.Note))
	}
	for _, h := range st.Hotspots {
		if h.Line == 0 {
			lines = append(lines, hotStyle.Render("⚑ ")+h.Q)
		}
	}
	if st.MayChange {
		lines = append(lines, delStyle.Render("may change after earlier comments"))
	}
	return append(lines, dimStyle.Render(strings.Repeat("─", max(m.mainWidth(), 1))))
}

type chatLine struct {
	at   time.Time
	text string
}

func (m *model) conversation() []chatLine {
	if m.review == nil {
		return nil
	}
	stepID := ""
	if m.step != nil {
		stepID = m.step.ID
	}
	var out []chatLine
	for _, msg := range m.review.Messages {
		if msg.Step == stepID {
			out = append(out, chatLine{msg.Time, agentStyle.Render("claude: ") + msg.Text})
		}
	}
	for _, e := range m.events {
		if e.Step != stepID || e.Kind == inbox.KindGoto {
			continue
		}
		text := e.Text
		if e.File != "" {
			text = strings.TrimSpace(fmt.Sprintf("%s:%s %s", e.File, e.Lines, text))
		}
		if e.Kind != inbox.KindMessage {
			text = "[" + e.Kind + "] " + text
		}
		out = append(out, chatLine{e.Time, dimStyle.Render("you: ") + text})
	}
	slices.SortStableFunc(out, func(a, b chatLine) int { return a.at.Compare(b.at) })
	return out
}

func (m *model) bottomLines() []string {
	limit := messageLines
	if m.step == nil {
		limit = max(m.height-4, 1)
	}
	var lines []string
	if chat := m.conversation(); len(chat) > 0 {
		var wrapped []string
		for _, c := range chat {
			wrapped = append(wrapped, strings.Split(ansi.Wrap(c.text, max(m.width, 10), ""), "\n")...)
		}
		if len(wrapped) > limit {
			wrapped = wrapped[len(wrapped)-limit:]
		}
		lines = append(lines, dimStyle.Render(strings.Repeat("─", max(m.width, 1))))
		lines = append(lines, wrapped...)
	}
	var last string
	switch {
	case m.composing:
		prompt := "› "
		if m.composeKind == inbox.KindSkip {
			prompt = "skip reason › "
		}
		if m.visual {
			if file, l, ok := m.selection(); ok {
				prompt = fmt.Sprintf("%s:%s › ", file, l)
			}
		}
		last = cursorStyle.Render(prompt) + string(m.input) + "█"
	case m.err != nil:
		last = delStyle.Render(m.err.Error())
	case m.status != "":
		last = dimStyle.Render(m.status)
	default:
		last = dimStyle.Render(hints)
	}
	return append(lines, last)
}

func (m *model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	if m.review == nil {
		msg := "waiting for gr init…"
		if m.err != nil {
			msg = m.err.Error()
		}
		return dimStyle.Render(msg)
	}
	if m.step == nil {
		return m.intakeView()
	}
	bottom := m.bottomLines()
	bodyH := max(m.height-len(bottom), 1)
	mw := m.mainWidth()

	main := m.header()
	for i := m.offset; len(main) < bodyH; i++ {
		if i < len(m.list) {
			main = append(main, m.renderRow(i, mw))
		} else {
			main = append(main, "")
		}
	}
	main = main[:bodyH]

	pw := m.planWidth()
	var plan []string
	if pw > 0 {
		plan = m.planLines(bodyH, pw)
	}
	out := make([]string, 0, m.height)
	for i, line := range main {
		line = fit(line, mw)
		if pw > 0 {
			line = fit(plan[i], pw-1) + dimStyle.Render("│") + line
		}
		out = append(out, line)
	}
	for _, line := range bottom {
		out = append(out, fit(line, m.width))
	}
	return strings.Join(out, "\n")
}

func fit(s string, w int) string {
	s = ansi.Truncate(s, w, "")
	if pad := w - ansi.StringWidth(s); pad > 0 {
		s += strings.Repeat(" ", pad)
	}
	return s
}

func (m *model) planOffset() int {
	bodyH := max(m.height-len(m.bottomLines()), 1)
	idx := m.review.StepIndex(m.review.Current)
	return max(0, idx-(bodyH-2))
}

func (m *model) planLines(h, w int) []string {
	title := "plan"
	if m.review.Round > 1 {
		title += fmt.Sprintf(" · round %d", m.review.Round)
	}
	lines := []string{boldStyle.Render(title)}
	for _, st := range m.review.Steps[m.planOffset():] {
		glyph := statusGlyph[st.Status]
		style := dimStyle
		if st.Status == state.StatusPending {
			style = lipgloss.NewStyle()
		}
		if st.ID == m.review.Current {
			glyph, style = "▶", cursorStyle
		}
		line := fmt.Sprintf("%s %s %s", glyph, st.ID, st.Title)
		if len(st.Hotspots) > 0 {
			line += " ⚑"
		}
		lines = append(lines, style.Render(ansi.Truncate(line, w-1, "…")))
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	return lines[:h]
}

func (m *model) gutter(i int) string {
	switch {
	case i == m.cursor:
		return cursorStyle.Render("▶")
	case m.selected(i):
		return selectStyle.Render("▌")
	}
	return " "
}

func (m *model) renderRow(i, w int) string {
	if m.useSplit() {
		return m.renderSplit(i, w)
	}
	return m.gutter(i) + renderUnified(m.rows[i])
}

func renderUnified(r Row) string {
	switch r.Kind {
	case RowFile:
		return fileStyle.Render(r.Text)
	case RowGap:
		return dimStyle.Render("      ⋯")
	case RowNote:
		return renderNote(r)
	}
	marker, num, text := " ", fmt.Sprintf("%4d", r.Line), r.Text
	switch r.Kind {
	case RowAdded:
		marker = addStyle.Render("+")
	case RowRemoved:
		marker, num, text = delStyle.Render("-"), "    ", delStyle.Render(r.Text)
	}
	if r.Hotspot {
		marker = hotStyle.Render("⚑")
	}
	return marker + dimStyle.Render(num+" │ ") + text
}

func renderNote(r Row) string {
	style, ok := noteStyles[r.NoteKind]
	if !ok {
		style = noteStyles["note"]
	}
	if r.Dim {
		style = dimStyle
	}
	return dimStyle.Render("       ┆ ") + style.Render(noteIcons[r.NoteKind]+" "+r.Text)
}

func (m *model) renderSplit(i, w int) string {
	r := m.split[i]
	if r.Full != nil {
		return m.gutter(i) + renderUnified(*r.Full)
	}
	side := (w - 2) / 2
	left := renderCell(r.Left, side, false)
	right := renderCell(r.Right, side, r.Hotspot)
	return m.gutter(i) + fit(left, side) + dimStyle.Render("┃") + right
}

func renderCell(c Cell, w int, hot bool) string {
	if c.Line == 0 && c.Text == "" {
		return strings.Repeat(" ", max(w, 0))
	}
	marker, text := " ", c.Text
	switch c.Kind {
	case RowAdded:
		marker = addStyle.Render("+")
	case RowRemoved:
		marker, text = delStyle.Render("-"), delStyle.Render(c.Text)
	}
	if hot {
		marker = hotStyle.Render("⚑")
	}
	return marker + dimStyle.Render(fmt.Sprintf("%4d │ ", c.Line)) + text
}

func (m *model) intakeView() string {
	title := "review " + m.review.ID
	if m.review.MR != nil {
		title += fmt.Sprintf(" · !%d %s", m.review.MR.IID, m.review.MR.Title)
	}
	top := []string{boldStyle.Render(title), dimStyle.Render("no plan yet — answer the agent below (c to write)")}
	bottom := m.bottomLines()
	out := make([]string, 0, m.height)
	for _, line := range top {
		out = append(out, fit(line, m.width))
	}
	for len(out)+len(bottom) < m.height {
		out = append(out, "")
	}
	for _, line := range bottom {
		out = append(out, fit(line, m.width))
	}
	return strings.Join(out[:min(len(out), max(m.height, len(top)))], "\n")
}
