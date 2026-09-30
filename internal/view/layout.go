package view

import (
	"cmp"
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/pltanton/guided-review/internal/inbox"
	"github.com/pltanton/guided-review/internal/state"
)

var (
	addStyle      = okTone.fg()
	delStyle      = badTone.fg()
	hotStyle      = warnTone.fg().Bold(true)
	dimStyle      = mutedTone.fg()
	boldStyle     = textTone.fg().Bold(true)
	fileStyle     = textTone.fg().Bold(true)
	fileInfoStyle = mutedTone.fg()
	cursorStyle   = accentTone.fg().Bold(true)
	agentStyle    = agentTone.fg().Bold(true)
	youStyle      = youTone.fg().Bold(true)
	buttonStyle   = textTone.fg().Background(surfaceTone.color())
	keyStyle      = accentTone.fg().Background(surfaceTone.color()).Bold(true)
	foldStyle     = badTone.fg().Faint(true)
	gapStyle      = blueTone.fg()
	fieldStyle    = mutedTone.fg().Background(surfaceTone.color())
	labelStyle    = mutedTone.fg().Bold(true)
	chapterStyle  = accentTone.fg()
	addEmphStyle  = textTone.fg().Background(addBgTone.color()).Bold(true)
	delEmphStyle  = textTone.fg().Background(delBgTone.color()).Bold(true)
)

const (
	hints      = "c message  h help  q quit"
	configHint = "~/.config/guided-review/config.yaml (gr config init)"
)

var spinner = []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")

func (m *model) spin() string {
	return string(spinner[m.frame%len(spinner)])
}

type button struct {
	label, key string
	press      func(*model) tea.Cmd
}

func (m *model) footerButtons() []button {
	km := m.keys()
	b := func(label, action string, press func(*model)) button {
		return button{label, km.key(action), func(m *model) tea.Cmd { press(m); return nil }}
	}
	btns := []button{
		b("✓ next", "next", (*model).next),
		b("✎ message", "message", func(m *model) { m.startCompose(inbox.KindMessage) }),
		b("? ask", "ask", func(m *model) { m.startCompose(inbox.KindAsk) }),
		b("↷ skip", "skip", func(m *model) { m.startCompose(inbox.KindSkip) }),
	}
	if m.review != nil && m.review.Publish != nil {
		btns = append(btns, button{"✓ finish", km.key("finish"), (*model).finish})
	}
	return btns
}

type span struct {
	from, to int
	b        button
}

func (m *model) footer() (string, []span) {
	line := m.agentStatus() + "  "
	tail := m.keys().key("help") + " help · " + m.keys().key("quit") + " quit"
	if h := m.cursorHint(); h != "" {
		tail = h
	}
	if m.status != "" {
		tail = m.status
	}
	if !m.mouse && m.status == "" {
		tail = hotStyle.Render("mouse off · "+m.keys().key("mouse")) + dimStyle.Render(" · "+tail)
	}
	if m.lspBusy != "" {
		tail = m.spin() + " lsp " + m.lspBusy + "…"
	}
	if typed := m.count + m.pendingKey; typed != "" {
		tail = typed
	}
	if m.step == nil {
		if m.status == "" {
			tail = hints
		}
		return dimStyle.Render(tail), nil
	}
	x := ansi.StringWidth(line)
	var spans []span
	for _, b := range m.footerButtons() {
		text := " " + b.label + " · " + b.key + " "
		w := ansi.StringWidth(text)
		spans = append(spans, span{x, x + w, b})
		line += buttonStyle.Render(" "+b.label+" · ") + keyStyle.Render(b.key+" ") + " "
		x += w + 1
	}
	return line + " " + dimStyle.Render(tail), spans
}

func (m *model) cursorHint() string {
	k, cur := m.keys(), m.current()
	switch {
	case cur.Kind == RowNote && cur.Ref > 0:
		del := k.key("delete-comment")
		return fmt.Sprintf("%s edit · %s%s delete · %s reply · %s fold",
			k.key("edit-comment"), del, del, k.key("message"), k.key("open"))
	case cur.Kind == RowNote && cur.NoteKind != "mr" && cur.NoteKind != "pending":
		return k.key("open") + " fold · " + k.key("details") + " details"
	case cur.Kind == RowNote:
		return k.key("open") + " fold"
	case cur.GapTo > 0 || cur.FoldKey != "":
		return k.key("open") + " open"
	}
	return ""
}

func (m *model) planWidth() int {
	if m.help || !m.showPlan || m.width < minPlanWidth || m.review == nil ||
		len(m.review.Steps) == 0 {
		return 0
	}
	if m.planW > 0 {
		return max(12, min(m.planW, m.width/2))
	}
	return min(maxPlanWidth, m.width/4)
}

func (m *model) mainWidth() int {
	return m.width - m.planWidth() - m.chatWidth()
}

func (m *model) chatWidth() int {
	w := max(36, m.width/4)
	if m.sideW > 0 {
		w = max(24, min(m.sideW, m.width*2/3))
	}
	if m.help || m.width-m.planWidth()-w < minCodeWidth {
		return 0
	}
	return w
}

func (m *model) chatScrollHint() string {
	return m.keys().key("chat-up") + "/" + m.keys().key("chat-down") + " scroll"
}

func (m *model) sideChatLines(h, w int) []string {
	rule := dimStyle.Render(strings.Repeat("─", max(w, 1)))
	lines := []string{
		labelStyle.Render("CHAT"),
		dimStyle.Render(m.chatScrollHint() + " · drag │ to resize"),
		rule,
	}
	km := m.keys()
	prompt := []string{rule, dimStyle.Render(fmt.Sprintf("› %s to write · %s without a line",
		km.key("message"), km.key("message-general")))}
	switch {
	case m.inputInSideChat():
		prompt = append([]string{rule}, m.promptLines(w)...)
	case len(m.answerOptions()) > 0:
		pills, _ := m.optionPills()
		prompt = []string{rule, pills}
	}
	lines = append(lines, window(m.chatLines(w, true), h-len(lines)-len(prompt), m.chatTop)...)
	for len(lines)+len(prompt) < h {
		lines = append(lines, "")
	}
	return append(lines, prompt...)[:h]
}

func (m *model) inputInSideChat() bool {
	return m.chatWidth() > 0 && m.composing && m.cmdMode == 0
}

func (m *model) bodyHeight() int {
	return max(m.height-len(m.bottomLines())-len(m.header())-1, 1)
}

func (m *model) header() []string {
	if m.step == nil {
		return nil
	}
	st := m.step
	if isExtra(st.ID) {
		return []string{
			boldStyle.Render(fmt.Sprintf("%s · %d files", st.Title, len(st.Hunks))),
			hotStyle.Render("outside the plan · current is " + m.review.Current +
				" — esc to return"),
			m.separator(),
		}
	}
	lines := []string{m.stepTitle(st)}
	if m.viewStep != "" {
		back := fmt.Sprintf("viewing %s · current is %s — esc to return", st.ID, m.review.Current)
		lines = append(lines, hotStyle.Render(back))
	}
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
	return append(lines, m.separator())
}

func (m *model) stepTitle(st *state.Step) string {
	pill := inkTone.fg().Background(accentTone.color()).Bold(true).Render(" " + st.ID + " ")
	left := pill + " " + boldStyle.Render(st.Title) +
		dimStyle.Render("  "+cmp.Or(st.Chapter, st.Kind))
	if st.Status != state.StatusPending {
		left += dimStyle.Render(" · " + string(st.Status))
	}
	if m.review.Round > 1 {
		left += dimStyle.Render(fmt.Sprintf(" · round %d", m.review.Round))
	}
	total, reviewed := len(m.review.Steps), 0
	for _, s := range m.review.Steps {
		if s.Status != state.StatusPending {
			reviewed++
		}
	}
	const barW = 12
	filled := barW * reviewed / max(total, 1)
	right := accentTone.fg().Render(strings.Repeat("━", filled)) +
		faintTone.fg().Render(strings.Repeat("━", barW-filled)) +
		dimStyle.Render(fmt.Sprintf(" %d/%d", m.review.StepIndex(st.ID)+1, total))
	gap := m.mainWidth() - ansi.StringWidth(left) - ansi.StringWidth(right) - 1
	if gap < 2 {
		return left
	}
	return left + strings.Repeat(" ", gap) + right
}

func (m *model) separator() string {
	w := max(m.mainWidth(), 1)
	if m.offset == 0 {
		return dimStyle.Render(strings.Repeat("─", w))
	}
	label := fmt.Sprintf("── ↑ %d lines above ", m.offset)
	return dimStyle.Render(label + strings.Repeat("─", max(w-ansi.StringWidth(label), 0)))
}

type chatLine struct {
	at   time.Time
	step string
	you  bool
	text string
}

func (m *model) conversation(all bool) []chatLine {
	if m.review == nil {
		return nil
	}
	stepID := ""
	if m.step != nil {
		stepID = m.step.ID
	}
	keep := func(step string, at time.Time) bool {
		return (all || step == stepID) && !at.Before(m.review.RoundStart)
	}
	var out []chatLine
	for _, msg := range m.review.Messages {
		if keep(msg.Step, msg.Time) {
			out = append(out, chatLine{at: msg.Time, step: msg.Step, text: msg.Text})
		}
	}
	for _, e := range m.events {
		if !keep(e.Step, e.Time) || e.Kind == inbox.KindGoto {
			continue
		}
		text := e.Text
		if e.File != "" {
			text = strings.TrimSpace(fmt.Sprintf("%s:%s %s", e.File, e.Lines, text))
		}
		if e.Kind != inbox.KindMessage {
			text = "[" + e.Kind + "] " + text
		}
		out = append(out, chatLine{at: e.Time, step: e.Step, you: true, text: text})
	}
	slices.SortStableFunc(out, func(a, b chatLine) int { return a.at.Compare(b.at) })
	for _, e := range m.events {
		if m.pending(e) && !m.agentIdle {
			thinking := hotStyle.Render(m.spin() + " thinking…")
			out = append(out, chatLine{at: e.Time, step: e.Step, text: thinking})
			break
		}
	}
	return out
}

func (m *model) chatLines(width int, all bool) []string {
	const gutter = "       " + "│ "
	roomy := all || m.step == nil
	var lines []string
	prevStep, prevYou := "\x00", false
	for i, c := range m.conversation(all) {
		named := roomy || i == 0 || c.you != prevYou
		if all && c.step != prevStep {
			named = true
			label := "intake"
			if st := m.stepByID(c.step); st != nil {
				label = st.ID + " " + st.Title
			} else if c.step != "" {
				label = c.step
			}
			if len(lines) > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, dimStyle.Render("── "+label+" ──"))
			prevStep = c.step
		} else if roomy && i > 0 {
			lines = append(lines, "")
		}
		style, name := agentStyle, "claude"
		if c.you {
			style, name = youStyle, "   you"
		}
		bar := style.Render("│")
		textW := max(width-ansi.StringWidth(gutter), 10)
		for j, l := range strings.Split(ansi.Wrap(c.text, textW, ""), "\n") {
			lead := "       " + bar + " "
			if j == 0 && named {
				lead = style.Render(name) + " " + bar + " "
			}
			lines = append(lines, lead+l)
		}
		prevYou = c.you
	}
	return lines
}

func window(lines []string, height, fromBottom int) []string {
	end := max(len(lines)-fromBottom, min(height, len(lines)))
	start := max(end-height, 0)
	return lines[start:end]
}

func (m *model) bottomLines() []string {
	limit := messageLines
	if m.step == nil {
		limit = max(m.height-4, 1)
	}
	var lines []string
	chat := m.chatLines(m.width, m.step == nil)
	if len(chat) > 0 && m.chatWidth() == 0 {
		label := "── chat · " + m.chatScrollHint() + " "
		label += strings.Repeat("─", max(m.width-ansi.StringWidth(label), 1))
		lines = append(lines, dimStyle.Render(ansi.Truncate(label, m.width, "")))
		lines = append(lines, window(chat, limit, m.chatTop)...)
		if pills, _ := m.optionPills(); len(m.answerOptions()) > 0 && !m.composing {
			lines = append(lines, pills)
		}
	}
	if m.inputInSideChat() {
		footer, _ := m.footer()
		return append(lines, footer)
	}
	return append(lines, m.promptLines(m.width)...)
}

func (m *model) promptLines(width int) []string {
	var last string
	switch {
	case m.composing && m.cmdMode != 0:
		pos := min(m.inputPos, len(m.input))
		last = cursorStyle.Render(string(m.cmdMode)) + m.inputWithCursor(pos)
		if m.status != "" {
			last += "   " + dimStyle.Render(m.status)
		}
		return wrapInput(last, width)
	case m.composing:
		prompt := "› "
		switch {
		case m.composeKind == inbox.KindSkip:
			prompt = "skip reason › "
		case m.composeKind == inbox.KindAsk:
			prompt = fmt.Sprintf("ask %s:%s › ", m.anchorFile, m.anchorLines)
		case m.composeKind == inbox.KindEdit:
			prompt = fmt.Sprintf("edit #%d › ", m.composeRef)
		case m.composeRef > 0:
			prompt = fmt.Sprintf("re #%d %s:%s › ", m.composeRef, m.anchorFile, m.anchorLines)
		case m.anchorFile != "":
			prompt = fmt.Sprintf("%s:%s › ", m.anchorFile, m.anchorLines)
		}
		hints := []string{"ctrl+r raw"}
		lead := cursorStyle.Render(prompt)
		if m.rawMode() {
			hints = []string{"tab severity", "ctrl+r via the agent"}
			tag := "RAW " + string(m.severity())
			if m.composeKind == inbox.KindEdit {
				tag = "RAW"
			} else if m.composeRef > 0 {
				prompt = fmt.Sprintf("%s:%s › ", m.anchorFile, m.anchorLines)
			}
			lead = delStyle.Bold(true).Render(tag) + " " + cursorStyle.Render(prompt)
		}
		switch m.composeKind {
		case inbox.KindAsk:
			hints = []string{"enter alone explains"}
		case inbox.KindSkip:
			hints = nil
		}
		if m.anchorFile != "" {
			hints = append(hints, "⌫ no line")
		}
		pos := min(m.inputPos, len(m.input))
		last = lead + m.inputWithCursor(pos) + dimStyle.Render("   "+strings.Join(hints, " · "))
		return wrapInput(last, width)
	case m.err != nil:
		last = delStyle.Render(m.err.Error())
	default:
		last, _ = m.footer()
	}
	return []string{last}
}

func (m *model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	if m.review == nil {
		msg := dimStyle.Render("waiting for the agent to start a review (gr init)…")
		if m.err != nil && !errors.Is(m.err, state.ErrNoReview) {
			msg = delStyle.Render(m.err.Error())
		}
		spin := hotStyle.Render(m.spin())
		lines := []string{boldStyle.Render("guided review"), "", spin + " " + msg}
		out := make([]string, m.height)
		for i, l := range lines {
			if j := (m.height-len(lines))/2 + i; j >= 0 && j < m.height {
				out[j] = strings.Repeat(" ", max((m.width-ansi.StringWidth(l))/2, 0)) + l
			}
		}
		return strings.Join(out, "\n")
	}
	if m.step == nil {
		return m.intakeView()
	}
	if m.preview != "" {
		return m.finishView()
	}
	bottom := m.bottomLines()
	bodyH := max(m.height-len(bottom), 1)
	mw := m.mainWidth()
	ph := min(max(bodyH*3/5, 6), bodyH-len(m.header()))
	if room := bodyH - ph - len(m.header()) - 1; m.popup != nil && room > 0 &&
		m.cursor-m.offset >= room {
		m.offset = m.cursor - room + 1
	}

	body := m.bodyHeight()
	below := ""
	if rest := len(m.lines) - (m.offset + body); rest > 0 {
		below = dimStyle.Render(fmt.Sprintf("   ↓ %d more lines below", rest))
	}
	panel := func(title, hint string, lines []string) []string {
		rule := dimStyle.Render(strings.Repeat("─", max(mw, 1)))
		return append([]string{boldStyle.Render(title), hint, rule}, lines...)
	}
	main := m.header()
	switch {
	case m.help:
		lines := m.helpLines(mw)
		main = panel("keys", dimStyle.Render("any key closes · j/k scroll · remap in "+configHint),
			lines[min(m.helpTop, len(lines)):])
	case m.loading != "":
		loading := fmt.Sprintf("%s loading %d files…", m.spin(), len(m.step.Hunks))
		main = append(main, "", "  "+hotStyle.Render(loading))
	}
	for i := m.offset; len(main) < bodyH-1; i++ {
		if i >= len(m.lines) || m.help {
			main = append(main, "")
			continue
		}
		row := m.renderRow(i, mw)
		if i == m.cursor && !m.useSplit() {
			if plain, ok := m.currentCode(); ok {
				from, to := wordBounds(plain, m.col)
				row = underline(row, codePrefix+from, codePrefix+to)
			}
		}
		line := fit(row, mw)
		switch {
		case i == m.cursor:
			line = paint(line, cursorTone)
		case m.selected(i):
			line = paint(line, selectTone)
		case m.lines[i].Kind == RowFile:
			line = paint(line, fileTone)
		}
		main = append(main, line)
	}
	main = append(main[:min(len(main), bodyH-1)], below)
	if m.popup != nil {
		box := m.popupLines(mw, ph)
		copy(main[len(main)-len(box):], box)
	}
	if m.pendingKey != "" {
		overlayRight(main[:len(main)-1], m.keyHints(m.pendingKey), mw)
	}

	pw := m.planWidth()
	var plan []string
	if pw > 0 {
		for _, e := range m.sidebar(bodyH, pw) {
			plan = append(plan, e.text)
		}
	}
	var chat []string
	if cw := m.chatWidth(); cw > 0 {
		chat = m.sideChatLines(len(main), cw-2)
	}
	out := make([]string, 0, m.height)
	for i, line := range main {
		line = fit(line, mw)
		if pw > 0 {
			line = fit(plan[i], pw-1) + faintTone.fg().Render("│") + line
		}
		if chat != nil {
			line += faintTone.fg().Render("│") + " " + chat[i]
		}
		out = append(out, line)
	}
	for _, line := range bottom {
		out = append(out, fit(line, m.width))
	}
	return strings.Join(out, "\n")
}

func wrapInput(s string, width int) []string {
	lines := strings.Split(ansi.Wrap(s, max(width, 10), ""), "\n")
	return lines[max(len(lines)-5, 0):]
}

func fit(s string, w int) string {
	s = ansi.Truncate(s, w, "")
	if pad := w - ansi.StringWidth(s); pad > 0 {
		s += strings.Repeat(" ", pad)
	}
	return s
}

type sideEntry struct {
	text string
	step string
	file string
}

func (m *model) planRows(h int) int {
	chapters := 0
	for i, st := range m.review.Steps {
		if st.Chapter != "" && (i == 0 || m.review.Steps[i-1].Chapter != st.Chapter) {
			chapters++
		}
	}
	return min(len(m.review.Steps)+chapters+len(m.extraSteps())+1, max(h/2, 2))
}

func (m *model) sidebar(h, w int) []sideEntry {
	iw := max(w-3, 8)
	row := func(left, right string) string {
		left = ansi.Truncate(left, max(iw-ansi.StringWidth(right)-1, 1), "…")
		gap := iw - ansi.StringWidth(left) - ansi.StringWidth(right)
		return left + strings.Repeat(" ", max(gap, 1)) + right
	}
	plain := func(line string) string { return " " + line }
	selected := func(line string) string {
		return paint(fit(accentTone.fg().Render("▌")+line, w-2), cursorTone)
	}

	reviewed, idW := 0, 0
	for _, st := range m.review.Steps {
		if st.Status != state.StatusPending {
			reviewed++
		}
		idW = max(idW, len(st.ID))
	}
	title := labelStyle.Render("PLAN")
	if m.review.Round > 1 {
		title += dimStyle.Render(fmt.Sprintf(" · round %d", m.review.Round))
	}
	count := fmt.Sprintf("%d/%d", reviewed, len(m.review.Steps))
	out := []sideEntry{{text: plain(row(title, dimStyle.Render(count)))}}

	rows := m.planRows(h)
	offset := max(0, m.review.StepIndex(m.review.Current)-(rows-2))
	chapter := ""
	for _, st := range m.review.Steps[offset:] {
		if st.Chapter != "" && st.Chapter != chapter && len(out) < rows-1 {
			head := chapterStyle.Render(strings.ToUpper(st.Chapter))
			out = append(out, sideEntry{text: plain(row(head, ""))})
		}
		chapter = st.Chapter
		if len(out) >= rows {
			break
		}
		glyph, glyphStyle, style := st.Status.Glyph(), dimStyle, dimStyle
		switch st.Status {
		case state.StatusPending:
			glyph, style = "○", textTone.fg()
		case state.StatusDone:
			glyphStyle = addStyle
		case state.StatusStale:
			glyphStyle = hotStyle
		}
		current := st.ID == m.review.Current
		if current {
			glyph, glyphStyle, style = "▶", cursorStyle, boldStyle
		}
		if m.viewStep != "" && m.step != nil && st.ID == m.step.ID {
			glyph, glyphStyle, style = "◆", hotStyle, hotStyle
		}
		left := glyphStyle.Render(glyph) + " " + dimStyle.Render(fmt.Sprintf("%-*s", idW, st.ID)) +
			" " + style.Render(st.Title)
		flag := ""
		if len(st.Hotspots) > 0 {
			flag = hotStyle.Render("⚑")
		}
		line := plain(row(left, flag))
		if current {
			line = selected(row(left, flag))
		}
		out = append(out, sideEntry{text: line, step: st.ID})
	}
	for _, st := range m.extraSteps() {
		if len(out) >= rows {
			break
		}
		glyph, style := "◇", dimStyle
		if m.step != nil && st.ID == m.step.ID {
			glyph, style = "◆", hotStyle
		}
		n := dimStyle.Render(fmt.Sprint(len(st.Hunks)))
		line := plain(row(style.Render(glyph+" "+st.Title), n))
		out = append(out, sideEntry{text: line, step: st.ID})
	}
	for len(out) < rows {
		out = append(out, sideEntry{})
	}

	files := m.stepFiles()
	if len(files) > 0 {
		n := dimStyle.Render(fmt.Sprint(len(files)))
		out = append(out, sideEntry{}, sideEntry{text: plain(row(labelStyle.Render("FILES"), n))})
		current := m.current().File
		prevDir := ""
		for i, f := range files {
			dir, base := path.Dir(f), path.Base(f)
			indent := ""
			if dir != "." {
				if dir != prevDir {
					out = append(out, sideEntry{text: plain(row(dimStyle.Render(dir+"/"), ""))})
				}
				indent = "  "
			}
			prevDir = dir
			stat := ""
			if rf := m.review.File(f); rf != nil {
				stat = addStyle.Render(fmt.Sprintf("+%d", rf.Added)) + " " +
					delStyle.Render(fmt.Sprintf("−%d", rf.Deleted))
			}
			here := m.focusFiles && i == m.fileCursor || !m.focusFiles && f == current
			name := textTone.fg().Render(indent + base)
			line := plain(row(name, stat))
			if here {
				line = selected(row(boldStyle.Render(indent+base), stat))
			}
			out = append(out, sideEntry{text: line, file: f})
		}
	}
	for len(out) < h {
		out = append(out, sideEntry{})
	}
	return out[:h]
}

func (m *model) renderRow(i, w int) string {
	if m.useSplit() {
		return m.renderSplit(i, w)
	}
	return renderUnified(m.animate(m.lines[i].Row))
}

func renderUnified(r Row) string {
	switch r.Kind {
	case RowFile:
		info := ""
		if r.FileInfo != "" {
			info = "   " + fileInfoStyle.Render(r.FileInfo)
		}
		return fileStyle.Render("▍ "+r.Text) + info
	case RowSpacer:
		return ""
	case RowGap:
		if r.GapTo == 0 {
			return dimStyle.Render("      ⋯")
		}
		n := r.GapTo - r.GapFrom + 1
		gap := fmt.Sprintf("      ⋯ %d hidden lines (%d–%d)", n, r.GapFrom, r.GapTo)
		return gapStyle.Render(gap) + dimStyle.Render("  · o to show")
	case RowNote:
		return renderNote(r)
	case RowFold:
		style := foldStyle
		if strings.HasPrefix(r.Text, "↕") {
			style = dimStyle
		}
		return "      " + style.Render(r.Text) + dimStyle.Render("  · o to show")
	}
	c := cellOf(r, r.Line)
	if r.Kind == RowRemoved {
		c.Line = 0
	}
	return renderCode(c, r.Hotspot)
}

const codePrefix = len("+1234 | ")

func renderCode(c Cell, hot bool) string {
	marker, num, text := " ", fmt.Sprintf("%4d", c.Line), c.Text
	if c.Line == 0 {
		num = "    "
	}
	switch c.Kind {
	case RowAdded:
		marker = addStyle.Render("+")
	case RowRemoved:
		marker, text = delStyle.Render("-"), delStyle.Render(c.Text)
	}
	switch {
	case c.Moved:
		marker, text = dimStyle.Render("↕"), dimStyle.Render(c.Plain)
	case c.Reformat:
		marker = dimStyle.Render("≈")
	case c.Emph != nil:
		text = renderEmph(c.Plain, c.Emph, c.Kind)
	}
	if hot {
		marker = hotStyle.Render("⚑")
	}
	sep := dimStyle.Render(" │ ")
	if c.Mark != "" {
		sep = " " + noteKinds[c.Mark].tone.fg().Render("┃") + " "
	}
	return marker + dimStyle.Render(num) + sep + text
}

func renderEmph(plain string, emph [][2]int, kind RowKind) string {
	base, strong := addStyle, addEmphStyle
	if kind == RowRemoved {
		base, strong = delStyle, delEmphStyle
	}
	runes := []rune(plain)
	var b strings.Builder
	pos := 0
	for _, e := range emph {
		from, to := min(e[0], len(runes)), min(e[1], len(runes))
		if from > pos {
			b.WriteString(base.Render(string(runes[pos:from])))
		}
		if to > from {
			b.WriteString(strong.Render(string(runes[from:to])))
		}
		pos = max(pos, to)
	}
	if pos < len(runes) {
		b.WriteString(base.Render(string(runes[pos:])))
	}
	return b.String()
}

const noteIndent = 1 + 7 + 2

var noteKinds = map[string]struct {
	tone  tone
	label string
}{
	"note":    {agentTone, "NOTE"},
	"spec":    {badTone, "SPEC"},
	"hotspot": {warnTone, "RISK"},
	"comment": {youTone, "YOU"},
	"mr":      {blueTone, "MR"},
	"pending": {warnTone, "…"},
}

func noteBadge(kind, label string) string {
	return " " + cmp.Or(label, noteKinds[kind].label) + " "
}

func noteKey(r Row) string {
	return fmt.Sprintf("%s:%d:%s:%d:%s", r.File, r.Line, r.NoteKind, r.Ref, r.NoteLabel)
}

func expandNotes(rows []Row, width int, folded map[string]bool) []Row {
	width = max(width, 20)
	out := make([]Row, 0, len(rows))
	for _, r := range rows {
		if r.Kind != RowNote {
			out = append(out, r)
			continue
		}
		badge := noteBadge(r.NoteKind, r.NoteLabel)
		pad := ansi.StringWidth(badge) + 1
		text := r.Text
		if r.Dim {
			text += " ✓"
		}
		lines := strings.Split(ansi.Wrap(text, max(width-pad, 10), ""), "\n")
		if folded[noteKey(r)] && len(lines) > 1 {
			lines = []string{ansi.Truncate(lines[0], max(width-pad-4, 10), "…") + " ▸"}
		}
		for i, line := range lines {
			row := r
			row.Text, row.NoteHead = line, i == 0
			out = append(out, row)
		}
	}
	return out
}

func renderNote(r Row) string {
	t, text := noteKinds[r.NoteKind].tone, textTone
	if r.Dim {
		t, text = faintTone, mutedTone
	}
	badge := noteBadge(r.NoteKind, r.NoteLabel)
	lead := strings.Repeat(" ", ansi.StringWidth(badge)+1)
	if r.NoteHead {
		pill := inkTone.fg().Background(t.color()).Bold(true)
		lead = pill.Render(badge) + " "
	}
	return "       " + t.fg().Render("▌") + " " + lead + text.fg().Render(r.Text)
}

func (m *model) renderSplit(i, w int) string {
	l := m.lines[i]
	if !l.Pair {
		return renderUnified(m.animate(l.Row))
	}
	side := (w - 2) / 2
	cell := func(c Cell, hot bool) string {
		if c.Line == 0 && c.Text == "" {
			return ""
		}
		return renderCode(c, hot)
	}
	right := cell(l.Right, l.Hotspot && l.Right.Line > 0)
	return fit(cell(l.Left, false), side) + dimStyle.Render("┃") + right
}

const intakeWidth = 100

func (m *model) intakeView() string {
	w := min(max(m.width-2, 20), intakeWidth)
	pad := strings.Repeat(" ", max((m.width-w)/2, 0))
	top := m.intakeTop(w)
	prompt := m.promptLines(w)
	if !m.composing && m.err == nil {
		field := fmt.Sprintf("› press %s or enter to answer the agent", m.keys().key("message"))
		line := fieldStyle.Render(fit(field, w))
		if len(m.answerOptions()) > 0 {
			line, _ = m.optionPills()
		}
		prompt = append([]string{line}, prompt...)
	}
	chatH := max(m.height-len(top)-len(prompt)-1, 1)
	chat := window(m.chatLines(w, false), chatH, m.chatTop)
	if len(chat) == 0 {
		chat = []string{dimStyle.Render("the agent is reading the MR; its questions show up here")}
	}
	rule := "── conversation "
	rule += strings.Repeat("─", max(w-ansi.StringWidth(rule), 0))
	lines := append(top, dimStyle.Render(rule))
	lines = append(lines, chat...)
	for len(lines)+len(prompt) < m.height {
		lines = append(lines, "")
	}
	lines = append(lines[:min(len(lines), max(m.height-len(prompt), 0))], prompt...)
	for i, l := range lines {
		lines[i] = pad + fit(l, w)
	}
	return strings.Join(lines, "\n")
}

func (m *model) intakeTop(w int) []string {
	r := m.review
	title := "guided review · " + r.ID
	if r.MR != nil {
		title = fmt.Sprintf("guided review · !%d %s", r.MR.IID, r.MR.Title)
	}
	var stages []string
	cur := 0
	switch {
	case len(r.Steps) == 0:
	case r.Publish != nil:
		cur = 2
	default:
		cur = 1
	}
	for i, name := range []string{"task & plan", "steps", "finish"} {
		if i == cur {
			stages = append(stages, cursorStyle.Render("● "+name))
		} else {
			stages = append(stages, dimStyle.Render("○ "+name))
		}
	}
	lines := []string{
		"",
		boldStyle.Render(ansi.Truncate(title, w, "…")),
		dimStyle.Render(strings.Repeat("─", w)),
		strings.Join(stages, dimStyle.Render("  ›  ")),
		"",
	}
	row := func(label, value string) {
		lines = append(lines, dimStyle.Render(fmt.Sprintf("%-9s", label))+value)
	}
	var added, deleted int
	tiers := map[state.Tier]int{}
	for _, f := range r.Files {
		added, deleted = added+f.Added, deleted+f.Deleted
		tiers[f.Tier]++
	}
	if len(r.Files) > 0 {
		plus, minus := fmt.Sprintf("+%d", added), fmt.Sprintf("−%d", deleted)
		row("changes", fmt.Sprintf("%d files  %s %s", len(r.Files),
			addStyle.Render(plus), delStyle.Render(minus))+
			dimStyle.Render(fmt.Sprintf("   core %d · boilerplate %d · generated %d",
				len(r.Files)-tiers[state.TierBoilerplate]-tiers[state.TierGenerated],
				tiers[state.TierBoilerplate], tiers[state.TierGenerated])))
	}
	if r.MR != nil {
		open := 0
		for _, d := range r.Discussions {
			if !d.Resolved {
				open++
			}
		}
		row("MR", fmt.Sprintf("%d open discussions", open))
	}
	if r.Round > 1 {
		row("round", fmt.Sprint(r.Round))
	}
	row("code", r.CodeDir(m.repo.Dir))
	status := m.agentStatus()
	if m.agentWaiting {
		status += dimStyle.Render(" — answer below")
	}
	return append(lines, "", status, "")
}

func (m *model) agentStatus() string {
	if m.agentWaiting {
		return addStyle.Render("● your turn")
	}
	if m.agentIdle {
		return delStyle.Render("○ agent stopped — answer in its window (a)")
	}
	text := "agent working"
	if m.review != nil && m.review.Progress != nil {
		text = m.review.Progress.Text
	}
	if !m.agentSince.IsZero() {
		text += " · " + m.clock().Sub(m.agentSince).Round(time.Second).String()
	}
	return hotStyle.Render(m.spin() + " " + text)
}

func (m *model) animate(r Row) Row {
	if r.Kind == RowNote && r.NoteKind == "pending" && r.NoteHead {
		r.NoteLabel = m.spin()
	}
	return r
}

func (m *model) inputWithCursor(pos int) string {
	return string(m.input[:pos]) + "█" + string(m.input[pos:])
}

func (m *model) keyHints(prefix string) []string {
	var rows [][2]string
	for _, a := range m.keys().actions {
		for _, k := range a.Keys {
			if next, ok := strings.CutPrefix(k, prefix+" "); ok {
				rows = append(rows, [2]string{next, a.Desc})
			}
		}
	}
	slices.SortFunc(rows, func(a, b [2]string) int { return strings.Compare(a[0], b[0]) })
	lines := []string{boldStyle.Render(prefix + "…")}
	for _, r := range rows {
		lines = append(lines, cursorStyle.Render(r[0])+"  "+r[1])
	}
	frame := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).
		BorderForeground(faintTone.color())
	box := frame.Padding(0, 1).Render(strings.Join(lines, "\n"))
	return strings.Split(box, "\n")
}

func overlayRight(lines, box []string, width int) {
	left := max(width-ansi.StringWidth(box[0]), 0)
	for i, b := range box {
		if j := len(lines) - len(box) + i; j >= 0 {
			lines[j] = fit(lines[j], left) + b
		}
	}
}
