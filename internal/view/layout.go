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
	chatStyle     = accentTone.fg().Bold(true)
)

const (
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
		b("next", "next", (*model).next),
		b("message", "message", func(m *model) { m.startCompose(inbox.KindMessage) }),
		b("ask", "ask", func(m *model) { m.startCompose(inbox.KindAsk) }),
		b("skip", "skip", func(m *model) { m.startCompose(inbox.KindSkip) }),
	}
	if n := m.pendingThreads(); n > 0 {
		label := fmt.Sprintf("replies %d", n)
		btns = append(btns, b(label, "replies", (*model).openThreads))
	}
	if m.review != nil && m.review.Publish != nil {
		btns = append(btns, button{"finish", km.key("finish"), (*model).finish})
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
			km := m.keys()
			tail = km.key("message") + " message  " + km.key("help") + " help  " +
				km.key("quit") + " quit"
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
	if m.notice != "" {
		return line + " " + hotStyle.Render(m.notice), spans
	}
	return line + " " + dimStyle.Render(tail), spans
}

func (m *model) cursorHint() string {
	k, cur := m.keys(), m.current()
	switch {
	case m.focusPlan || m.focusFiles:
		return m.panelHint()
	case cur.Kind == RowNote && cur.Ref > 0:
		del := k.key("delete-comment")
		return fmt.Sprintf("%s edit · %s%s delete · %s reply · %s fold",
			k.key("edit-comment"), del, del, k.key("message"), k.key("open"))
	case cur.Kind == RowNote && agentKinds[cur.NoteKind]:
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

const sideChatTop = 3

func (m *model) sidePrompt(w int) []string {
	rule := dimStyle.Render(strings.Repeat("─", max(w, 1)))
	km := m.keys()
	switch {
	case m.inputInSideChat():
		return append([]string{rule}, m.promptLines(w)...)
	case len(m.answerOptions()) > 0:
		pills, _ := m.optionPills(w)
		return append([]string{rule}, pills...)
	}
	return []string{rule, dimStyle.Render(fmt.Sprintf("› %s to write · %s without a line",
		km.key("message"), km.key("message-general")))}
}

func (m *model) sideChatLines(h, w int) []string {
	title, hint := labelStyle.Render("CHAT"), m.chatScrollHint()+" · drag │ to resize"
	if m.chatFocus {
		title, hint = chatStyle.Render("CHAT"), m.chatFocusHint()
	}
	lines := []string{title, dimStyle.Render(hint), dimStyle.Render(strings.Repeat("─", max(w, 1)))}
	prompt := m.sidePrompt(w)
	chatH := max(h-len(lines)-len(prompt), 0)
	lines = append(lines, m.chatWindow(m.chatRows(w, true), chatH, w)...)
	for len(lines)+len(prompt) < h {
		lines = append(lines, "")
	}
	lines = append(lines, prompt...)
	return lines[max(len(lines)-h, 0):]
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
	bar, w := chapterStyle.Render("▌ "), max(m.mainWidth()-4, 20)
	switch intro := m.chapterIntro(st); {
	case st.Intro != "":
		lines = append(lines, bar+chapterStyle.Bold(true).Render(cmp.Or(st.Chapter, "chapter")))
		for _, l := range strings.Split(ansi.Wrap(st.Intro, w, ""), "\n") {
			lines = append(lines, bar+textTone.fg().Render(l))
		}
	case intro != "":
		first, _, _ := strings.Cut(intro, "\n")
		line := chapterStyle.Bold(true).Render(st.Chapter) + dimStyle.Render(" · "+first)
		lines = append(lines, bar+ansi.Truncate(line, w, "…"))
	}
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
	if m.review.Filling && st.Message == "" && !isExtra(st.ID) {
		left += hotStyle.Render("  " + m.spin() + " agent is writing notes")
	}
	if total, unseen := m.changedRows(), m.unseen(); total > 0 && unseen == 0 {
		left += addStyle.Render("  ✓ seen")
	} else if total > 0 {
		left += dimStyle.Render(fmt.Sprintf("  seen %d/%d", total-unseen, total))
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
	live bool
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
			out = append(out, chatLine{at: e.Time, step: e.Step, live: true, text: thinking})
			break
		}
	}
	return out
}

type chatRow struct {
	text      string
	msg, hard int
	source    string
}

func (m *model) chatLines(width int, all bool) []string {
	var out []string
	for _, r := range m.chatRows(width, all) {
		out = append(out, r.text)
	}
	return out
}

func (m *model) chatRows(width int, all bool) []chatRow {
	const gutter = "       " + "│ "
	roomy := all || m.step == nil
	var lines []chatRow
	gap := func(text string) { lines = append(lines, chatRow{text: text, msg: -1}) }
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
				gap("")
			}
			gap(dimStyle.Render("── " + label + " ──"))
			prevStep = c.step
		} else if roomy && i > 0 {
			gap("")
		}
		style, name := agentStyle, "claude"
		if c.you {
			style, name = youStyle, "   you"
		}
		bar := style.Render("│")
		textW := max(width-ansi.StringWidth(gutter), 10)
		msg, first := i, true
		if c.live {
			msg = -1
		}
		for h, hard := range strings.Split(c.text, "\n") {
			for _, l := range strings.Split(ansi.Wrap(hard, textW, ""), "\n") {
				lead := "       " + bar + " "
				if first && named {
					lead = style.Render(name) + " " + bar + " "
				}
				first = false
				lines = append(lines, chatRow{text: lead + l, msg: msg, hard: h, source: hard})
			}
		}
		prevYou = c.you
	}
	return lines
}

func windowRange(n, height, fromBottom int) (start, end int) {
	end = max(n-fromBottom, min(height, n))
	return max(end-height, 0), end
}

func (m *model) bottomLines() []string {
	limit := messageLines
	if m.step == nil {
		limit = max(m.height-4, 1)
	}
	var lines []string
	chat := m.chatRows(m.width, m.step == nil)
	if len(chat) > 0 && m.chatWidth() == 0 {
		label, style := "── chat · "+m.chatScrollHint()+" ", dimStyle
		if m.chatFocus {
			label, style = "── chat · "+m.chatFocusHint()+" ", chatStyle
		}
		label += strings.Repeat("─", max(m.width-ansi.StringWidth(label), 1))
		lines = append(lines, style.Render(ansi.Truncate(label, m.width, "")))
		lines = append(lines, m.chatWindow(chat, limit, m.width)...)
		if len(m.answerOptions()) > 0 && !m.composing {
			pills, _ := m.optionPills(m.width)
			lines = append(lines, pills...)
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
		hint := ""
		if m.status != "" {
			hint = "   " + dimStyle.Render(m.status)
		}
		return m.inputLines(cursorStyle.Render(string(m.cmdMode)), hint, width)
	case m.composing:
		bar := accentTone
		if m.rawMode() {
			bar = badTone
		}
		lines := m.inputLines(bar.fg().Render("▌")+" ", "", width)
		for i, l := range lines {
			lines[i] = paint(fit(l, width), surfaceTone)
		}
		return append(lines, m.composeStatus(width))
	case m.err != nil:
		last = delStyle.Render(m.err.Error())
	default:
		last, _ = m.footer()
	}
	return []string{last}
}

type composeHint struct{ key, desc string }

func (m *model) composeMode() (
	badge string, style lipgloss.Style, anchor string, hints []composeHint,
) {
	loc := ""
	if m.anchorFile != "" {
		loc = path.Base(m.anchorFile) + ":" + m.anchorLines
	}
	badge, style, anchor = "MSG", keyStyle, loc
	hints = []composeHint{{"enter", "send"}}
	switch {
	case m.composeKind == inbox.KindSkip:
		badge, hints[0].desc = "SKIP", "skip"
	case m.composeKind == kindThreadReply:
		badge, anchor, hints[0].desc = "REPLY", m.threadLabel(), "reply, thread stays open"
	case m.composeThread != "":
		badge, anchor = "THREAD", m.threadLabel()
	case m.composeKind == inbox.KindAsk:
		badge, hints = "ASK", append(hints, composeHint{"enter", "alone explains"})
	case m.rawMode():
		style, hints[0].desc = delStyle.Bold(true).Background(surfaceTone.color()), "save"
		if m.composeKind == inbox.KindEdit {
			badge = fmt.Sprintf("RAW EDIT #%d", m.composeRef)
		} else {
			badge, hints = "RAW "+string(m.severity()), append(hints, composeHint{"tab", "severity"})
		}
		hints = append(hints, composeHint{"ctrl+r", "via agent"})
	case m.composeKind == inbox.KindEdit:
		badge, hints[0].desc = fmt.Sprintf("EDIT #%d", m.composeRef), "save"
		hints = append(hints, composeHint{"ctrl+r", "raw"})
	default:
		if m.composeRef > 0 {
			anchor = strings.TrimSpace(fmt.Sprintf("re #%d %s", m.composeRef, loc))
		}
		hints = append(hints, composeHint{"ctrl+r", "raw"})
	}
	hints = append(hints, composeHint{"alt+enter", "new line"})
	return badge, style, anchor, append(hints, composeHint{"esc", "cancel"})
}

func (m *model) threadLabel() string {
	if m.review == nil {
		return ""
	}
	for _, d := range m.review.Discussions {
		switch {
		case d.ID != m.composeThread:
		case d.File != "":
			return fmt.Sprintf("%s:%d", path.Base(d.File), d.Line)
		case d.Author != "":
			return "@" + d.Author
		}
	}
	return ""
}

func (m *model) composeStatus(width int) string {
	badge, style, anchor, hints := m.composeMode()
	left := style.Render(" " + badge + " ")
	if m.interrupted {
		left += hotStyle.Background(surfaceTone.color()).Render("paused ")
	}
	if anchor != "" {
		left += buttonStyle.Render(anchor + " ")
	}
	lw := ansi.StringWidth(left)
	if lw >= width {
		return ansi.Truncate(left, width, "…")
	}
	for n := len(hints); n > 0; n-- {
		parts := make([]string, n)
		for i, h := range hints[:n] {
			parts[i] = cursorStyle.Render(h.key) + dimStyle.Render(" "+h.desc)
		}
		right := strings.Join(parts, dimStyle.Render(" · "))
		if gap := width - lw - ansi.StringWidth(right); gap >= 2 {
			return left + strings.Repeat(" ", gap) + right
		}
	}
	return left
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
	if m.threads {
		return m.threadsView()
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
	if room := bodyH - ph - len(m.header()) - 1; m.popup != nil && room > 0 {
		m.offset = max(m.offset, m.topFor(m.cursor, room))
	}

	if !m.help {
		m.markShown()
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
	shown := m.offset
	for i := m.offset; len(main) < bodyH-1; i++ {
		if i >= len(m.lines) || m.help {
			main = append(main, "")
			continue
		}
		rows := m.renderRows(i, mw)
		if i == m.cursor && !m.useSplit() {
			if plain, ok := m.currentCode(); ok {
				from, to := wordBounds(plain, m.col)
				m.underlineWord(rows, from, to, mw)
			}
		}
		if room := bodyH - 1 - len(main); len(rows) <= room {
			shown = i + 1
		} else {
			rows = rows[:room]
		}
		for _, row := range rows {
			main = append(main, m.paintRow(i, fit(row, mw)))
		}
	}
	below := ""
	if rest := len(m.lines) - shown; rest > 0 && !m.help {
		below = dimStyle.Render(fmt.Sprintf("   ↓ %d more lines below", rest))
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

func (m *model) paintRow(i int, line string) string {
	switch l := m.lines[i]; {
	case i == m.cursor:
		return paint(line, cursorTone)
	case m.selected(i):
		return paint(line, selectTone)
	case l.Kind == RowFile:
		return paint(line, fileTone)
	case l.Kind == RowNote && l.Dim:
		return paint(line, surfaceTone)
	case l.Kind == RowNote:
		return paint(line, noteKinds[l.NoteKind].bg)
	case l.Pair:
	case l.Kind == RowAdded:
		return paint(line, addLineTone)
	case l.Kind == RowRemoved:
		return paint(line, delLineTone)
	}
	return line
}

func fit(s string, w int) string {
	s = ansi.Truncate(s, w, "")
	if pad := w - ansi.StringWidth(s); pad > 0 {
		s += strings.Repeat(" ", pad)
	}
	return s
}

type sideZone int

const (
	zoneNone sideZone = iota
	zonePlan
	zoneFiles
)

type sideEntry struct {
	text    string
	step    string
	file    string
	chapter string
	zone    sideZone
}

type rowFunc func(left, right string) string

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
	header := func(label string, focused bool, top, shown, n int, count string) string {
		title := labelStyle.Render(label)
		if focused {
			title = cursorStyle.Render(label)
		}
		more := ""
		if top > 0 {
			more += fmt.Sprintf("↑%d ", top)
		}
		if rest := n - top - shown; rest > 0 {
			more += fmt.Sprintf("↓%d ", rest)
		}
		return plain(row(title, dimStyle.Render(more+count)))
	}

	items := m.planItems()
	if m.focusPlan {
		m.planCursor = max(0, min(m.planCursor, len(items)-1))
	}
	plan := m.planEntries(items, row, plain, selected)
	files, fileAt := m.fileEntries(row, plain, selected)

	filesNeed := 0
	if len(files) > 0 {
		filesNeed = len(files) + 2
	}
	planH := min(1+len(plan), max(h-filesNeed, h/2, 2))
	filesH := min(filesNeed, h-planH)
	if filesH < 3 {
		filesH = 0
	}

	at := m.planAnchor(items)
	if m.focusPlan {
		at = m.planCursor
	}
	rows := max(planH-1, 0)
	key := fmt.Sprint(rows, " ", plan[at].step, "#", plan[at].chapter)
	top := scrollWindow(&m.planTop, &m.planFollow, key, m.focusPlan, at, rows, len(plan))
	reviewed := 0
	for _, st := range m.review.Steps {
		if st.Status != state.StatusPending {
			reviewed++
		}
	}
	label := "PLAN"
	if m.review.Round > 1 {
		label += fmt.Sprintf(" · round %d", m.review.Round)
	}
	count := fmt.Sprintf("%d/%d", reviewed, len(m.review.Steps))
	shown := min(rows, len(plan)-top)
	title := header(label, m.focusPlan, top, shown, len(plan), count)
	out := append([]sideEntry{{text: title, zone: zonePlan}}, plan[top:top+shown]...)
	for len(out) < planH {
		out = append(out, sideEntry{zone: zonePlan})
	}

	if filesH > 0 {
		rows := filesH - 2
		key := fmt.Sprint(rows, " ", files[fileAt].file)
		top := scrollWindow(&m.fileTop, &m.fileFollow, key, m.focusFiles, fileAt, rows, len(files))
		shown := min(rows, len(files)-top)
		title := header("FILES", m.focusFiles, top, shown, len(files), fmt.Sprint(len(m.stepFiles())))
		out = append(out, sideEntry{}, sideEntry{text: title, zone: zoneFiles})
		out = append(out, files[top:top+shown]...)
	}
	out = append(out, m.flowEntries(row, plain)...)
	for len(out) < h {
		out = append(out, sideEntry{})
	}
	return out[:h]
}

func (m *model) planEntries(
	items []planItem, row rowFunc, plain, selected func(string) string,
) []sideEntry {
	idW := 0
	for _, st := range m.review.Steps {
		idW = max(idW, len(st.ID))
	}
	viewed := ""
	if m.viewStep != "" && m.step != nil {
		viewed = m.step.ID
	}
	out := make([]sideEntry, 0, len(items))
	for i, it := range items {
		cursor := m.focusPlan && i == m.planCursor
		st := it.st
		switch {
		case it.head:
			glyph := "▸"
			if it.open {
				glyph = "▾"
			}
			stat := dimStyle.Render(fmt.Sprintf("%d/%d", it.done, it.total))
			if it.hot > 0 {
				stat += " " + hotStyle.Render(fmt.Sprintf("⚑%d", it.hot))
			}
			left := chapterStyle.Render(glyph + " " + strings.ToUpper(it.chapter))
			line := plain(row(left, stat))
			if cursor || !m.focusPlan && it.current && !it.open {
				line = selected(row(left, stat))
			}
			out = append(out, sideEntry{text: line, chapter: it.chapter, zone: zonePlan})
		case it.extra:
			glyph, style := "◇", dimStyle
			if m.step != nil && st.ID == m.step.ID {
				glyph, style = "◆", hotStyle
			}
			n := dimStyle.Render(fmt.Sprint(len(st.Hunks)))
			left := style.Render(glyph + " " + st.Title)
			line := plain(row(left, n))
			if cursor {
				line = selected(row(left, n))
			}
			out = append(out, sideEntry{text: line, step: st.ID, zone: zonePlan})
		default:
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
			if st.ID == viewed {
				glyph, glyphStyle, style = "◆", hotStyle, hotStyle
			}
			indent := ""
			if it.chapter != "" {
				indent = " "
			}
			left := indent + glyphStyle.Render(glyph) + " " +
				dimStyle.Render(fmt.Sprintf("%-*s", idW, st.ID)) + " " + style.Render(st.Title)
			flag := ""
			if len(st.Hotspots) > 0 {
				flag = hotStyle.Render("⚑")
			}
			line := plain(row(left, flag))
			if cursor || !m.focusPlan && current {
				line = selected(row(left, flag))
			}
			out = append(out, sideEntry{text: line, step: st.ID, zone: zonePlan})
		}
	}
	return out
}

func (m *model) fileEntries(row rowFunc, plain, selected func(string) string) ([]sideEntry, int) {
	files := m.stepFiles()
	if m.focusFiles {
		m.fileCursor = max(0, min(m.fileCursor, len(files)-1))
	}
	current := m.current().File
	var out []sideEntry
	at, prevDir := 0, ""
	for i, f := range files {
		dir, base := path.Dir(f), path.Base(f)
		indent := ""
		if dir != "." {
			if dir != prevDir {
				text := plain(row(dimStyle.Render(dir+"/"), ""))
				out = append(out, sideEntry{text: text, zone: zoneFiles})
			}
			indent = "  "
		}
		prevDir = dir
		stat := ""
		if rf := m.review.File(f); rf != nil {
			stat = addStyle.Render(fmt.Sprintf("+%d", rf.Added)) + " " +
				delStyle.Render(fmt.Sprintf("−%d", rf.Deleted))
		}
		line := plain(row(textTone.fg().Render(indent+base), stat))
		if m.focusFiles && i == m.fileCursor || !m.focusFiles && f == current {
			line = selected(row(boldStyle.Render(indent+base), stat))
			at = len(out)
		}
		out = append(out, sideEntry{text: line, file: f, zone: zoneFiles})
	}
	return out, at
}

func (m *model) flowEntries(row rowFunc, plain func(string) string) []sideEntry {
	if len(m.flow) == 0 {
		return nil
	}
	n := dimStyle.Render(fmt.Sprint(len(m.flow)))
	out := []sideEntry{{}, {text: plain(row(labelStyle.Render("FLOW"), n))}}
	calls := func(arrow string, names []string) {
		var seen []string
		for _, n := range names {
			n, _, _ = strings.Cut(n, "(")
			if slices.Contains(seen, n) {
				continue
			}
			seen = append(seen, n)
			if len(seen) > maxFlowCalls {
				continue
			}
			cls, method := "", n
			if i := strings.LastIndex(n, "."); i > 0 {
				cls, method = n[:i], n[i+1:]
			}
			text := dimStyle.Render("  "+arrow+" ") + textTone.fg().Render(method)
			if cls != "" {
				text += faintTone.fg().Render(" · " + cls)
			}
			out = append(out, sideEntry{text: plain(row(text, ""))})
		}
		if extra := len(seen) - maxFlowCalls; extra > 0 {
			more := dimStyle.Render(fmt.Sprintf("    +%d more", extra))
			out = append(out, sideEntry{text: plain(row(more, ""))})
		}
	}
	for _, f := range m.flow {
		out = append(out, sideEntry{text: plain(row(boldStyle.Render(f.name), ""))})
		calls("←", f.in)
		calls("→", f.out)
	}
	return out
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
	gutter, _, text := codeParts(c, hot)
	return gutter + text
}

func emphasize(text string, emph [][2]int, kind RowKind) string {
	strong, line := addBgTone, addLineTone
	if kind == RowRemoved {
		strong, line = delBgTone, delLineTone
	}
	for _, e := range emph {
		text = markRange(text, e[0], e[1], bgSeq(strong)+"\x1b[1m", "\x1b[22m"+bgSeq(line))
	}
	return text
}

const (
	noteIndent   = 1 + 7 + 2
	maxFlowCalls = 4
)

var noteKinds = map[string]struct {
	tone, bg tone
	label    string
}{
	"note":    {agentTone, tone{"#E8F6F8", "#162529"}, "NOTE"},
	"spec":    {badTone, tone{"#FBEDED", "#2A1A1D"}, "SPEC"},
	"hotspot": {warnTone, tone{"#FBF4E4", "#292316"}, "RISK"},
	"comment": {youTone, tone{"#FAEBF3", "#291827"}, "YOU"},
	"mr":      {blueTone, tone{"#EAF0FB", "#172033"}, "MR"},
	"pending": {warnTone, tone{"#FBF4E4", "#292316"}, "…"},
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
	t, text := noteKinds[r.NoteKind].tone, noteTextTone
	if r.Dim {
		t, text = faintTone, mutedTone
	}
	badge := noteBadge(r.NoteKind, r.NoteLabel)
	lead := strings.Repeat(" ", ansi.StringWidth(badge)+1)
	if r.NoteHead {
		pill := inkTone.fg().Background(t.color()).Bold(true)
		lead = pill.Render(badge) + " "
	}
	return "       " + t.fg().Render("▌") + " " + lead + text.fg().Italic(true).Render(r.Text)
}

func (m *model) renderSplit(l line, w int) (rows []string) {
	lw, rw := m.splitWidths(w)
	cell := func(c Cell, hot bool, w int) []string {
		if c.Line == 0 && c.Text == "" {
			return nil
		}
		return m.codeRows(c, hot, w)
	}
	tint := func(s string, c Cell, w int) string {
		switch c.Kind {
		case RowAdded:
			return paint(fit(s, w), addLineTone)
		case RowRemoved:
			return paint(fit(s, w), delLineTone)
		}
		return fit(s, w)
	}
	left := cell(l.Left, false, lw)
	right := cell(l.Right, l.Hotspot && l.Right.Line > 0, rw)
	at := func(side []string, k int) string {
		if k < len(side) {
			return side[k]
		}
		return ""
	}
	for k := range max(len(left), len(right), 1) {
		rows = append(rows, tint(at(left, k), l.Left, lw)+dimStyle.Render("┃")+
			tint(at(right, k), l.Right, rw))
	}
	return rows
}

const intakeWidth = 100

func (m *model) intakeView() string {
	w := min(max(m.width-2, 20), intakeWidth)
	pad := strings.Repeat(" ", max((m.width-w)/2, 0))
	top := m.intakeTop(w)
	prompt := m.intakePrompt(w)
	chatH := max(m.height-len(top)-len(prompt)-1, 1)
	chat := m.chatWindow(m.chatRows(w, false), chatH, w)
	if len(chat) == 0 {
		chat = []string{dimStyle.Render("the agent is reading the MR; its questions show up here")}
	}
	rule, style := "── conversation ", dimStyle
	if m.chatFocus {
		rule, style = "── conversation · "+m.chatFocusHint()+" ", chatStyle
	}
	rule += strings.Repeat("─", max(w-ansi.StringWidth(rule), 0))
	lines := append(top, style.Render(rule))
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

func (m *model) intakePrompt(w int) []string {
	prompt := m.promptLines(w)
	if m.composing || m.err != nil {
		return prompt
	}
	field := fmt.Sprintf("› press %s or enter to answer the agent", m.keys().key("message"))
	lines := []string{fieldStyle.Render(fit(field, w))}
	if len(m.answerOptions()) > 0 {
		lines, _ = m.optionPills(w)
	}
	return append(lines, prompt...)
}

func (m *model) intakeTop(w int) []string {
	r := m.review
	title := "guided review · " + r.ID
	if r.MR != nil {
		title = "guided review · " + r.MR.Label() + " " + r.MR.Title
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

func withCursor(rs []rune, pos int) string {
	if pos >= len(rs) || rs[pos] == '\n' {
		return string(rs[:min(pos, len(rs))]) + "█"
	}
	under := lipgloss.NewStyle().Reverse(true).Render(string(rs[pos]))
	return string(rs[:pos]) + under + string(rs[pos+1:])
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
	return hintBox(prefix, rows)
}

func hintBox(prefix string, rows [][2]string) []string {
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
