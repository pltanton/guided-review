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
	addStyle, delStyle, hotStyle, dimStyle, boldStyle, fileStyle     lipgloss.Style
	fileInfoStyle, cursorStyle, agentStyle, youStyle, buttonStyle    lipgloss.Style
	keyStyle, foldStyle, gapStyle, fieldStyle, labelStyle, chatStyle lipgloss.Style
	chapterStyle                                                     lipgloss.Style
)

func init() { restyle(defaultTones.noteBg) }

func restyle(noteBg map[string]tone) {
	addStyle, delStyle = okTone.fg(), badTone.fg()
	hotStyle, dimStyle = warnTone.fg().Bold(true), mutedTone.fg()
	boldStyle, fileStyle = textTone.fg().Bold(true), textTone.fg().Bold(true)
	fileInfoStyle, cursorStyle = mutedTone.fg(), accentTone.fg().Bold(true)
	agentStyle, youStyle = agentTone.fg().Bold(true), youTone.fg().Bold(true)
	buttonStyle = textTone.fg().Background(surfaceTone.color())
	keyStyle = accentTone.fg().Background(surfaceTone.color()).Bold(true)
	foldStyle, gapStyle = badTone.fg().Faint(true), blueTone.fg()
	fieldStyle = mutedTone.fg().Background(surfaceTone.color())
	labelStyle, chapterStyle, chatStyle = mutedTone.fg().Bold(true), accentTone.fg(),
		accentTone.fg().Bold(true)
	noteKinds = map[string]noteKind{
		"note":    {agentTone, noteBg["note"], "NOTE"},
		"spec":    {badTone, noteBg["spec"], "SPEC"},
		"hotspot": {warnTone, noteBg["hotspot"], "RISK"},
		"comment": {youTone, noteBg["comment"], "YOU"},
		"mr":      {blueTone, noteBg["mr"], "MR"},
		"pending": {warnTone, noteBg["pending"], "…"},
	}
	severityTones = map[string]tone{
		"blocker": badTone, "major": warnTone, "minor": agentTone, "nit": mutedTone,
	}
}

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
		b("comment", "act", (*model).startComment),
		b("ask", "ask", func(m *model) { m.startCompose(inbox.KindAsk) }),
		b("skip", "skip", func(m *model) { m.startCompose(kindSkip) }),
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
	tail := m.keys().key("help") + " keys"
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
	line = m.modeBadge() + " " + line
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

func (m *model) modeBadge() string {
	if !m.visual {
		return buttonStyle.Render(" NORMAL ")
	}
	n := max(m.cursor, m.anchor) - min(m.cursor, m.anchor) + 1
	badge := inkTone.fg().Background(accentTone.color()).Bold(true).Render(" VISUAL ")
	return badge + keyStyle.Render(fmt.Sprintf(" %d lines ", n))
}

func (m *model) cursorHint() string {
	k, cur := m.keys(), m.current()
	switch {
	case m.focusPlan || m.focusFiles:
		return m.panelHint()
	case m.visual:
		return fmt.Sprintf("%s comment · %s ask · %s copy · esc cancel",
			k.key("act"), k.key("ask"), k.key("yank"))
	case cur.Kind == RowNote && cur.Ref > 0:
		del := k.key("delete-comment")
		return fmt.Sprintf("%s edit · %s%s delete · %s reply · %s fold",
			k.key("edit-comment"), del, del, k.key("message"), k.key("open"))
	case cur.Thread != "":
		return k.key("act") + " open the thread · " + k.key("replies") + " all your threads"
	case cur.Risk > 0:
		return fmt.Sprintf("%s details · %s check off", k.key("act"), k.key("check-risk"))
	case cur.Kind == RowNote && agentKinds[cur.NoteKind]:
		return k.key("act") + " details · " + k.key("open") + " fold"
	case cur.Kind == RowNote:
		return k.key("open") + " fold"
	case cur.Kind == RowGap || cur.FoldKey != "":
		return k.key("open") + " open"
	}
	return ""
}

func (m *model) mainWidth() int {
	return m.width - m.chatWidth()
}

const (
	collapsedChatRows = 6
	collapsedBottom   = 2
)

func (m *model) chatOpen() bool {
	return m.chatting || m.chatFocus || m.composing && m.cmdMode == 0 && !m.inlineCompose()
}

func (m *model) chatWidth() int {
	if m.step == nil || m.width-max(36, m.width/4) < minCodeWidth {
		return 0
	}
	small := max(30, m.width/6)
	if !m.chatOpen() {
		return small
	}
	w := max(48, m.width*2/5)
	if m.sideW > 0 {
		w = max(24, min(m.sideW, m.width*2/3))
	}
	return max(small, min(w, m.width-minCodeWidth))
}

func (m *model) chatScrollHint() string {
	return m.keys().key("chat-up") + "/" + m.keys().key("chat-down") + " scroll"
}

func (m *model) sidePrompt(w int) []string {
	rule := dimStyle.Render(strings.Repeat("─", max(w, 1)))
	km := m.keys()
	switch {
	case m.inputInSideChat() && len(m.answerOptions()) > 0:
		pills, _ := m.optionPills(w)
		return append(append([]string{rule}, pills...), m.promptLines(w)...)
	case m.inputInSideChat():
		return append([]string{rule}, m.promptLines(w)...)
	case len(m.answerOptions()) > 0:
		pills, _ := m.optionPills(w)
		return append([]string{rule}, pills...)
	}
	return []string{rule, dimStyle.Render(fmt.Sprintf("› %s chat · %s without a line",
		km.key("message"), km.key("message-general")))}
}

type sideChat struct {
	files  []string
	header string
	rows   []chatRow
	chatH  int
	prompt []string
}

func (m *model) sideChatLayout(h, w int) sideChat {
	var c sideChat
	for _, e := range m.sideFiles(h, w+2) {
		c.files = append(c.files, e.text)
	}
	row, plain, _ := sideRows(w + 2)
	label, hint := labelStyle.Render("CHAT"), m.keys().key("message")+" opens"
	switch {
	case m.chatFocus:
		label, hint = cursorStyle.Render("CHAT"), m.chatFocusHint()
	case m.chatOpen():
		label, hint = cursorStyle.Render("CHAT"), "esc closes"
	}
	c.header = plain(row(label, dimStyle.Render(hint)))
	c.prompt = m.sidePrompt(w)
	c.rows = m.chatRows(w, true)
	c.chatH = max(h-len(c.files)-1-len(c.prompt), 0)
	if !m.chatOpen() {
		c.chatH = min(c.chatH, collapsedChatRows)
	}
	return c
}

func (m *model) sideChatLines(h, w int) []string {
	c := m.sideChatLayout(h, w)
	chat := m.chatWindow(c.rows, c.chatH, w)
	block := append(append([]string{c.header}, chat...), c.prompt...)
	lines := c.files
	for len(lines)+len(block) < h {
		lines = append(lines, "")
	}
	lines = append(lines, block...)
	return lines[max(len(lines)-h, 0):]
}

func (m *model) inputInSideChat() bool {
	return m.chatWidth() > 0 && m.composing && m.cmdMode == 0 && !m.inlineCompose()
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
	return append(lines, m.separator())
}

func (m *model) stepTitle(st *state.Step) string {
	pill := inkTone.fg().Background(accentTone.color()).Bold(true).Render(" " + st.ID + " ")
	left := pill + " " + chapterStyle.Render(cmp.Or(st.Chapter, st.Kind)) +
		dimStyle.Render(" › ") + boldStyle.Render(st.Title)
	if st.MayChange {
		left += delStyle.Render("  may change")
	}
	if st.Status != state.StatusPending {
		left += dimStyle.Render(" · " + string(st.Status))
	}
	if m.review.Round > 1 {
		left += dimStyle.Render(fmt.Sprintf(" · round %d", m.review.Round))
	}
	if m.review.Filling && st.Message == "" && !isExtra(st.ID) {
		left += hotStyle.Render("  " + m.spin() + " agent is writing notes")
	}
	if total, unseen := len(m.changedKeys()), m.unseen(); total > 0 && unseen == 0 {
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
	const barW, maxDots = 12, 20
	var right string
	if total <= maxDots {
		right = accentTone.fg().Render(strings.Repeat("●", reviewed)) +
			faintTone.fg().Render(strings.Repeat("○", total-reviewed))
	} else {
		filled := barW * reviewed / max(total, 1)
		right = accentTone.fg().Render(strings.Repeat("━", filled)) +
			faintTone.fg().Render(strings.Repeat("━", barW-filled))
	}
	right += dimStyle.Render(fmt.Sprintf(" %d/%d", m.review.StepIndex(st.ID)+1, total))
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
		if !keep(e.Step, e.Time) {
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
			for l := range strings.SplitSeq(ansi.Wrap(hard, textW, ""), "\n") {
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
	if !m.chatOpen() {
		limit = collapsedBottom
	}
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
		if len(m.answerOptions()) > 0 && (!m.composing || m.chatOpen()) {
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
	case m.composing && !m.inlineCompose():
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
	case m.composeKind == kindSkip:
		badge, hints[0].desc = "SKIP", "skip"
	case m.composeKind == kindThreadReply:
		badge, anchor, hints[0].desc = "REPLY", m.threadLabel(), "reply, thread stays open"
	case m.composeThread != "":
		badge, anchor = "THREAD", m.threadLabel()
	case m.composeKind == inbox.KindAsk:
		badge, hints = "ASK", append(hints, composeHint{"enter", "alone explains"})
		if m.inlineCompose() {
			hints = append(hints, composeHint{"tab", "comment"})
		}
	case m.rawMode() && m.composeKind == inbox.KindEdit:
		badge, hints[0].desc = fmt.Sprintf("EDIT #%d", m.composeRef), "save"
		hints = append(hints, composeHint{"tab", "ai edit"})
	case m.rawMode():
		badge, hints[0].desc = "COMMENT "+string(m.severity()), "save"
		hints = append(hints, composeHint{"tab", "ai comment"}, composeHint{"S-tab", "severity"})
		if m.sugOn {
			hints = append(hints, composeHint{"C-s", "comment ⇄ suggestion"})
		} else {
			hints = append(hints, composeHint{"C-s", "suggestion"})
		}
	case m.composeKind == inbox.KindEdit:
		badge = fmt.Sprintf("AI EDIT #%d", m.composeRef)
		if m.inlineCompose() {
			hints = append(hints, composeHint{"tab", "edit"})
		}
	default:
		if m.composeRef > 0 {
			anchor = strings.TrimSpace(fmt.Sprintf("re #%d %s", m.composeRef, loc))
		}
		if m.chatting {
			badge, anchor = "CHAT", cmp.Or(m.chatTopic, anchor)
			return badge, style, anchor, append(hints,
				composeHint{"C-j", "new line"}, composeHint{"esc", "leave the chat"})
		}
		if m.inlineCompose() {
			badge, hints = "AI COMMENT", append(hints, composeHint{"tab", "ask"})
		}
	}
	hints = append(hints, composeHint{"C-j", "new line"})
	return badge, style, anchor, append(hints, composeHint{"esc", "cancel"})
}

func (m *model) threadLabel() string {
	if m.review == nil {
		return ""
	}
	d := m.review.Discussion(m.composeThread)
	switch {
	case d == nil:
		return ""
	case d.File != "":
		return fmt.Sprintf("%s:%d", path.Base(d.File), d.Line)
	}
	return "@" + d.Author
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
	if m.listW != m.mainWidth() {
		m.relist()
	}
	if !m.newsChecked {
		m.newsChecked = true
		m.showNewsOnce()
	}
	bottom := m.bottomLines()
	bodyH := max(m.height-len(bottom), 1)
	mw := m.mainWidth()

	if !m.blocked() {
		m.markShown()
	}
	main := m.header()
	switch {
	case m.loading != "":
		loading := fmt.Sprintf("%s loading %d files…", m.spin(), len(m.step.Hunks))
		main = append(main, "", "  "+hotStyle.Render(loading))
	}
	shown := m.offset
	for i := m.offset; len(main) < bodyH-1; i++ {
		if i >= len(m.lines) {
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
		for k, row := range rows {
			rows[k] = m.paintRow(i, fit(row, mw))
		}
		if i == m.inlineAt && m.inlineCompose() {
			rows = append(rows, m.composerRows(mw)...)
		}
		if room := bodyH - 1 - len(main); len(rows) <= room {
			shown = i + 1
		} else {
			rows = rows[:room]
		}
		main = append(main, rows...)
	}
	below := ""
	if rest := len(m.lines) - shown; rest > 0 {
		below = dimStyle.Render(fmt.Sprintf("   ↓ %d more lines below", rest))
	}
	main = append(main[:min(len(main), bodyH-1)], below)
	if m.pendingKey != "" {
		overlayRight(main[:len(main)-1], m.keyHints(m.pendingKey), mw)
	}

	var chat []string
	if cw := m.chatWidth(); cw > 0 {
		chat = m.sideChatLines(len(main), cw-2)
	}
	out := make([]string, 0, m.height)
	for i, line := range main {
		line = fit(line, mw)
		if chat != nil {
			line += faintTone.fg().Render("│") + " " + chat[i]
		}
		out = append(out, line)
	}
	for _, line := range bottom {
		out = append(out, fit(line, m.width))
	}
	m.drawModal(out)
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

type sideEntry struct {
	text  string
	file  string
	files bool
}

type rowFunc func(left, right string) string

func sideRows(w int) (row rowFunc, plain, selected func(string) string) {
	iw := max(w-3, 8)
	row = func(left, right string) string {
		left = ansi.Truncate(left, max(iw-ansi.StringWidth(right)-1, 1), "…")
		gap := iw - ansi.StringWidth(left) - ansi.StringWidth(right)
		return left + strings.Repeat(" ", max(gap, 1)) + right
	}
	plain = func(line string) string { return " " + line }
	selected = func(line string) string {
		return paint(fit(accentTone.fg().Render("▌")+line, w-2), cursorTone)
	}
	return row, plain, selected
}

func scrollMarks(top, shown, n int) string {
	more := ""
	if top > 0 {
		more += fmt.Sprintf("↑%d ", top)
	}
	if rest := n - top - shown; rest > 0 {
		more += fmt.Sprintf("↓%d ", rest)
	}
	return more
}

func (m *model) sideFiles(h, w int) []sideEntry {
	if m.step == nil || m.review == nil {
		return nil
	}
	row, plain, selected := sideRows(w)
	files, at := m.fileEntries(row, plain, selected)
	if len(files) == 0 {
		return nil
	}
	flow := m.flowEntries(row, plain)
	limit := max(h/2, 4)
	if !m.chatOpen() {
		limit = max(h-collapsedChatRows-4, 4)
	}
	rows := min(len(files), limit-2)
	key := fmt.Sprint(rows, " ", files[at].file)
	top := scrollWindow(&m.fileTop, &m.fileFollow, key, m.focusFiles, at, rows, len(files))
	shown := min(rows, len(files)-top)
	label := labelStyle.Render("FILES")
	if m.focusFiles {
		label = cursorStyle.Render("FILES")
	}
	count := fmt.Sprint(len(m.stepFiles())) + " · " + m.step.ID
	title := plain(row(label, dimStyle.Render(scrollMarks(top, shown, len(files))+count)))
	out := append([]sideEntry{{text: title, files: true}}, files[top:top+shown]...)
	if len(out)+len(flow)+1 <= limit {
		out = append(out, flow...)
	}
	return append(out, sideEntry{})
}

func (m *model) planEntries(
	items []planItem, row rowFunc, plain, selected func(string) string,
) []string {
	idW := 0
	for _, st := range m.review.Steps {
		idW = max(idW, len(st.ID))
	}
	viewed := ""
	if m.viewStep != "" && m.step != nil {
		viewed = m.step.ID
	}
	out := make([]string, 0, len(items))
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
			out = append(out, line)
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
			out = append(out, line)
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
			out = append(out, line)
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
				out = append(out, sideEntry{text: text, files: true})
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
		out = append(out, sideEntry{text: line, file: f, files: true})
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
	case RowEnd:
		return addStyle.Render("  ✓ ") + dimStyle.Render(r.Text)
	case RowGap:
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
	return ""
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

type noteKind struct {
	tone, bg tone
	label    string
}

var noteKinds map[string]noteKind

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
		lines := strings.Split(ansi.Wrap(r.Text, max(width-pad, 10), ""), "\n")
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
		t, text = okTone, mutedTone
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

func (m *model) cardFrame(maxW int) (x, w, h int) {
	w = min(max(m.width-6, 20), maxW)
	return max((m.width-w-4)/2, 0) + 2, w, max(m.height-2, 1)
}

func (m *model) card(x, w int, title string, body []string) string {
	box := modalBox(w+4, m.height, title, "", body)
	pad := strings.Repeat(" ", max(x-2, 0))
	for i := range box {
		box[i] = pad + box[i]
	}
	return strings.Join(box, "\n")
}

func (m *model) intakeView() string {
	x, w, h := m.cardFrame(intakeWidth)
	top := m.intakeTop(w)
	prompt := m.intakePrompt(w)
	chatH := max(h-len(top)-len(prompt)-1, 1)
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
	for len(lines)+len(prompt) < h {
		lines = append(lines, "")
	}
	lines = append(lines[:min(len(lines), max(h-len(prompt), 0))], prompt...)
	return m.card(x, w, m.intakeTitle(), lines)
}

func (m *model) intakePrompt(w int) []string {
	prompt := m.promptLines(w)
	if m.composing && len(m.answerOptions()) > 0 {
		pills, _ := m.optionPills(w)
		return append(pills, prompt...)
	}
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

func (m *model) intakeTitle() string {
	if r := m.review; r.MR != nil {
		return "guided review · " + r.MR.Label() + " " + r.MR.Title
	}
	return "guided review · " + m.review.ID
}

func (m *model) intakeTop(w int) []string {
	r := m.review
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
	lines := []string{strings.Join(stages, dimStyle.Render("  ›  ")), ""}
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
		return delStyle.Render("○ agent stopped — answer in its window (" + m.keys().key("agent") + ")")
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
