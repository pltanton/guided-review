package view

import (
	"cmp"
	"fmt"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/aplotnikov/guided-review/internal/inbox"
	"github.com/aplotnikov/guided-review/internal/state"
)

var (
	addStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	delStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	hotStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true)
	dimStyle      = lipgloss.NewStyle().Faint(true)
	boldStyle     = lipgloss.NewStyle().Bold(true)
	fileStyle     = lipgloss.NewStyle().Bold(true).Foreground(white)
	fileInfoStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
	cursorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true)
	agentStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true)
	buttonStyle   = lipgloss.NewStyle().Background(lipgloss.Color("8")).Foreground(white)
	foldStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Faint(true)
	gapStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("4"))
	addEmphStyle  = emphStyle("22")
	delEmphStyle  = emphStyle("52")
)

func emphStyle(bg string) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(white).Background(lipgloss.Color(bg)).Bold(true)
}

const white = lipgloss.Color("15")

const (
	hints      = "c message  h help  q quit"
	configHint = "~/.config/guided-review/config.yaml (gr config init)"
	cursorBg   = "236"
	selectBg   = "238"
	fileBg     = "24"
)

var spinner = []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")

func (m *model) spin() string {
	return string(spinner[m.frame%len(spinner)])
}

type button struct {
	label, key string
	press      func(*model)
}

func (m *model) footerButtons() []button {
	km := m.keys()
	b := func(label, action string, press func(*model)) button {
		return button{label, km.key(action), press}
	}
	btns := []button{
		b("✓ next", "next", (*model).next),
		b("✎ message", "message", func(m *model) { m.startCompose(inbox.KindMessage) }),
		b("? explain", "explain", (*model).explain),
		b("↷ skip", "skip", func(m *model) { m.startCompose(inbox.KindSkip) }),
	}
	if m.review != nil && m.review.Publish != nil {
		btns = append(btns, b("⬆ publish", "publish", (*model).publish))
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
	if m.status != "" {
		tail = m.status
	}
	if m.lspBusy != "" {
		tail = m.spin() + " lsp " + m.lspBusy + "…"
	}
	if m.step == nil {
		if m.status == "" {
			tail = hints
		}
		return line + dimStyle.Render(tail), nil
	}
	x := ansi.StringWidth(line)
	var spans []span
	for _, b := range m.footerButtons() {
		text := " " + b.label + " · " + b.key + " "
		w := ansi.StringWidth(text)
		spans = append(spans, span{x, x + w, b})
		line += buttonStyle.Render(text) + " "
		x += w + 1
	}
	return line + " " + dimStyle.Render(tail), spans
}

func (m *model) planWidth() int {
	if m.help || !m.showPlan || m.width < minPlanWidth || m.review == nil ||
		len(m.review.Steps) == 0 {
		return 0
	}
	return min(maxPlanWidth, m.width/4)
}

func (m *model) mainWidth() int {
	return m.width - m.planWidth()
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
	n := m.review.StepIndex(st.ID) + 1
	title := fmt.Sprintf("%s %d/%d %s · %s", st.ID, n, len(m.review.Steps), st.Kind, st.Title)
	if st.Status != state.StatusPending {
		title += " [" + string(st.Status) + "]"
	}
	if m.review.Round > 1 {
		title += fmt.Sprintf("  round %d", m.review.Round)
	}
	lines := []string{boldStyle.Render(title)}
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
	keep := func(step string) bool { return all || step == stepID }
	var out []chatLine
	for _, msg := range m.review.Messages {
		if keep(msg.Step) {
			text := agentStyle.Render("claude: ") + msg.Text
			out = append(out, chatLine{msg.Time, msg.Step, text})
		}
	}
	for _, e := range m.events {
		if !keep(e.Step) || e.Kind == inbox.KindGoto {
			continue
		}
		text := e.Text
		if e.File != "" {
			text = strings.TrimSpace(fmt.Sprintf("%s:%s %s", e.File, e.Lines, text))
		}
		if e.Kind != inbox.KindMessage {
			text = "[" + e.Kind + "] " + text
		}
		out = append(out, chatLine{e.Time, e.Step, dimStyle.Render("you: ") + text})
	}
	slices.SortStableFunc(out, func(a, b chatLine) int { return a.at.Compare(b.at) })
	for _, e := range m.events {
		if m.pending(e) && !m.agentIdle {
			thinking := hotStyle.Render(m.spin() + " thinking…")
			out = append(out, chatLine{e.Time, e.Step, agentStyle.Render("claude: ") + thinking})
			break
		}
	}
	return out
}

func (m *model) chatLines(width int, all bool) []string {
	var lines []string
	prev := "\x00"
	for _, c := range m.conversation(all) {
		if all && c.step != prev {
			label := "intake"
			if st := m.stepByID(c.step); st != nil {
				label = st.ID + " " + st.Title
			} else if c.step != "" {
				label = c.step
			}
			lines = append(lines, dimStyle.Render("── "+label+" ──"))
			prev = c.step
		}
		lines = append(lines, strings.Split(ansi.Wrap(c.text, max(width, 10), ""), "\n")...)
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
	switch {
	case m.step == nil:
		limit = max(m.height-4, 1)
	case m.chatSize == 1:
		limit = max(m.height/2, messageLines)
	}
	var lines []string
	if m.chatSize < 2 {
		if chat := m.chatLines(m.width, m.chatSize == 1 || m.step == nil); len(chat) > 0 {
			label := strings.Repeat("─", max(m.width, 1))
			if m.chatSize == 0 {
				label = "── " + m.keys().key("chat") + " enlarges the chat " +
					strings.Repeat("─", max(m.width-24, 1))
			}
			lines = append(lines, dimStyle.Render(ansi.Truncate(label, m.width, "")))
			lines = append(lines, window(chat, limit, m.chatTop)...)
		}
	}
	var last string
	switch {
	case m.composing && m.cmdMode != 0:
		pos := min(m.inputPos, len(m.input))
		last = cursorStyle.Render(string(m.cmdMode)) + m.inputWithCursor(pos)
		if m.status != "" {
			last += "   " + dimStyle.Render(m.status)
		}
		return append(lines, wrapInput(last, m.width)...)
	case m.composing:
		prompt := "› "
		switch {
		case m.composeKind == inbox.KindSkip:
			prompt = "skip reason › "
		case m.composeKind == inbox.KindEdit:
			prompt = fmt.Sprintf("edit #%d › ", m.composeRef)
		case m.composeRef > 0:
			prompt = fmt.Sprintf("re #%d %s:%s › ", m.composeRef, m.anchorFile, m.anchorLines)
		case m.anchorFile != "":
			prompt = fmt.Sprintf("%s:%s › ", m.anchorFile, m.anchorLines)
		}
		pos := min(m.inputPos, len(m.input))
		hint := ""
		if m.anchorFile != "" {
			hint = dimStyle.Render("   ctrl+x: no line")
		}
		last = cursorStyle.Render(prompt) + m.inputWithCursor(pos) + hint
		return append(lines, wrapInput(last, m.width)...)
	case m.err != nil:
		last = delStyle.Render(m.err.Error())
	default:
		last, _ = m.footer()
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
	switch k := m.keys(); {
	case m.preview != "":
		lines := strings.Split(strings.TrimRight(m.preview, "\n"), "\n")
		title := "publish preview"
		if m.review.MR != nil {
			title += fmt.Sprintf(" → !%d", m.review.MR.IID)
		}
		main = panel(title,
			hotStyle.Render("P publishes all of this to the MR · esc cancels · j/k scroll"),
			lines[min(m.previewTop, len(lines)):])
	case m.help:
		lines := m.helpLines(mw)
		main = panel("keys", dimStyle.Render("any key closes · j/k scroll · remap in "+configHint),
			lines[min(m.helpTop, len(lines)):])
	case m.chatSize == 2:
		hint := fmt.Sprintf("j/k scroll · %s write · %s / esc back to the code",
			k.key("message"), k.key("chat"))
		chat := window(m.chatLines(mw, true), bodyH-4, m.chatTop)
		main = panel("chat", dimStyle.Render(hint), chat)
	case m.loading != "":
		loading := fmt.Sprintf("%s loading %d files…", m.spin(), len(m.step.Hunks))
		main = append(main, "", "  "+hotStyle.Render(loading))
	}
	for i := m.offset; len(main) < bodyH-1; i++ {
		if i >= len(m.lines) || m.preview != "" || m.help || m.chatSize == 2 {
			main = append(main, "")
			continue
		}
		row := m.renderRow(i, mw)
		if i == m.cursor && !m.useSplit() {
			if plain, ok := m.currentCode(); ok {
				r := m.lines[i].Row
				r.Text, r.Emph, r.Moved = underlineWord(plain, m.col), nil, false
				row = renderUnified(r)
			}
		}
		line := fit(row, mw)
		switch {
		case i == m.cursor:
			line = paint(line, cursorBg)
		case m.selected(i):
			line = paint(line, selectBg)
		case m.lines[i].Kind == RowFile:
			line = paint(line, fileBg)
		}
		main = append(main, line)
	}
	main = append(main[:min(len(main), bodyH-1)], below)
	if m.popup != nil {
		ph := min(max(bodyH*3/5, 6), bodyH-len(m.header()))
		box := m.popupLines(mw, ph)
		copy(main[len(main)-len(box):], box)
	}

	pw := m.planWidth()
	var plan []string
	if pw > 0 {
		for _, e := range m.sidebar(bodyH, pw) {
			plan = append(plan, e.text)
		}
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
	return min(len(m.review.Steps)+len(m.extraSteps())+1, max(h/2, 2))
}

func (m *model) sidebar(h, w int) []sideEntry {
	title := "plan"
	if m.review.Round > 1 {
		title += fmt.Sprintf(" · round %d", m.review.Round)
	}
	rows := m.planRows(h)
	idx := m.review.StepIndex(m.review.Current)
	offset := max(0, idx-(rows-2))
	entry := func(style lipgloss.Style, text string) sideEntry {
		return sideEntry{text: style.Render(ansi.Truncate(text, w-1, "…"))}
	}
	out := []sideEntry{{text: boldStyle.Render(title)}}
	for _, st := range m.review.Steps[offset:] {
		if len(out) >= rows {
			break
		}
		glyph := st.Status.Glyph()
		style := dimStyle
		if st.Status == state.StatusPending {
			style = lipgloss.NewStyle()
		}
		if st.ID == m.review.Current {
			glyph, style = "▶", cursorStyle
		}
		if m.viewStep != "" && m.step != nil && st.ID == m.step.ID {
			glyph, style = "◆", hotStyle
		}
		line := fmt.Sprintf("%s %s %s", glyph, st.ID, st.Title)
		if len(st.Hotspots) > 0 {
			line += " ⚑"
		}
		e := entry(style, line)
		e.step = st.ID
		out = append(out, e)
	}
	for _, st := range m.extraSteps() {
		if len(out) >= rows {
			break
		}
		glyph, style := "◇", dimStyle
		if m.step != nil && st.ID == m.step.ID {
			glyph, style = "◆", hotStyle
		}
		e := entry(style, fmt.Sprintf("%s %s · %d files", glyph, st.Title, len(st.Hunks)))
		e.step = st.ID
		out = append(out, e)
	}
	for len(out) < rows {
		out = append(out, sideEntry{})
	}

	files := m.stepFiles()
	if len(files) > 0 {
		out = append(out, sideEntry{}, sideEntry{text: boldStyle.Render("files")})
		current := m.current().File
		prevDir := ""
		for i, f := range files {
			dir, base := path.Dir(f), path.Base(f)
			indent := ""
			if dir != "." {
				if dir != prevDir {
					out = append(out, entry(dimStyle, dir+"/"))
				}
				indent = "  "
			}
			prevDir = dir
			line := indent + base
			if rf := m.review.File(f); rf != nil {
				line += dimStyle.Render(fmt.Sprintf(" +%d -%d", rf.Added, rf.Deleted))
			}
			style := lipgloss.NewStyle()
			switch {
			case m.focusFiles && i == m.fileCursor:
				line, style = "▶ "+line, cursorStyle
			case !m.focusFiles && f == current:
				style = cursorStyle
			}
			e := entry(style, line)
			e.file = f
			out = append(out, e)
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
	return marker + dimStyle.Render(num+" │ ") + text
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

func paint(line, bg string) string {
	seq := "\x1b[48;5;" + bg + "m"
	return seq + strings.ReplaceAll(line, "\x1b[0m", "\x1b[0m"+seq) + "\x1b[0m"
}

const noteIndent = 1 + 7 + 2

var noteKinds = map[string]struct{ color, label string }{
	"note":    {"6", "NOTE"},
	"spec":    {"1", "SPEC"},
	"hotspot": {"3", "RISK"},
	"comment": {"5", "YOU"},
	"mr":      {"4", "MR"},
	"pending": {"3", "…"},
}

func noteBadge(kind, label string) string {
	return " " + cmp.Or(label, noteKinds[kind].label) + " "
}

func expandNotes(rows []Row, width int) []Row {
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
		for i, line := range strings.Split(ansi.Wrap(text, max(width-pad, 10), ""), "\n") {
			row := r
			row.Text, row.NoteHead = line, i == 0
			out = append(out, row)
		}
	}
	return out
}

func renderNote(r Row) string {
	color := lipgloss.Color(noteKinds[r.NoteKind].color)
	if r.Dim {
		color = lipgloss.Color("8")
	}
	body := lipgloss.NewStyle().Foreground(color)
	badge := noteBadge(r.NoteKind, r.NoteLabel)
	lead := strings.Repeat(" ", ansi.StringWidth(badge)+1)
	if r.NoteHead {
		lead = body.Reverse(true).Bold(true).Render(badge) + " "
	}
	return "       " + body.Render("▌") + " " + lead + body.Render(r.Text)
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

func (m *model) intakeView() string {
	title := "review " + m.review.ID
	if m.review.MR != nil {
		title += fmt.Sprintf(" · !%d %s", m.review.MR.IID, m.review.MR.Title)
	}
	top := []string{
		boldStyle.Render(title),
		dimStyle.Render("no plan yet — answer the agent below (c to write)"),
	}
	if !m.agentWaiting {
		top = append(top, "", "  "+m.agentStatus())
	}
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
