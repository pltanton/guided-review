package view

import (
	"cmp"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/pltanton/guided-review/internal/inbox"
	"github.com/pltanton/guided-review/internal/state"
)

func (m *model) startCompose(kind string) {
	if m.review == nil || m.step == nil && kind != inbox.KindMessage {
		return
	}
	m.composing, m.composeKind, m.input, m.inputPos = true, kind, nil, 0
	m.composeRef, m.anchorFile, m.anchorLines, m.composeThread = 0, "", "", ""
	switch kind {
	case inbox.KindMessage:
		m.anchorFile, m.anchorLines, m.composeRef = m.anchorAt()
	case inbox.KindAsk:
		if m.anchorFile, m.anchorLines, _ = m.anchorAt(); m.anchorFile == "" {
			m.composing = false
			m.status = "nothing to ask about here: put the cursor on code"
		}
	}
}

func (m *model) anchorAt() (file, lines string, ref int) {
	cur := m.current()
	switch {
	case m.visual:
		file, lines, _ = m.selection()
	case cur.Ref > 0:
		file, lines, ref = cur.File, fmt.Sprint(cur.Line), cur.Ref
	case cur.File != "" && cur.Line > 0:
		file, lines = cur.File, fmt.Sprint(cur.Line)
	}
	return file, lines, ref
}

func (m *model) startEdit() {
	ref := m.current().Ref
	if ref == 0 || m.review == nil {
		m.status = "put the cursor on one of your comments to edit it"
		return
	}
	for _, c := range m.review.Comments {
		if c.ID == ref {
			m.composing, m.composeKind, m.composeRef = true, inbox.KindEdit, ref
			m.anchorFile, m.anchorLines = "", ""
			m.input = []rune(c.Body)
			m.inputPos = len(m.input)
			return
		}
	}
}

func (m *model) handleCompose(msg tea.KeyMsg) tea.Cmd {
	if m.cmdMode != 0 {
		switch msg.Type {
		case tea.KeyEnter:
			return m.submitCmd()
		case tea.KeyTab:
			m.complete()
			return nil
		case tea.KeyUp:
			m.historyMove(-1)
			return nil
		case tea.KeyDown:
			m.historyMove(1)
			return nil
		case tea.KeyEsc:
			m.composing, m.input, m.cmdMode = false, nil, 0
			return nil
		}
	}
	switch msg.Type {
	case tea.KeyEsc:
		m.composing, m.input, m.composeThread = false, nil, ""
		m.resume()
	case tea.KeyCtrlJ:
		m.insert([]rune{'\n'})
	case tea.KeyEnter:
		if msg.Alt {
			m.insert([]rune{'\n'})
			return nil
		}
		defer m.resume()
		text := strings.TrimSpace(string(m.input))
		thread := m.composeThread
		m.composing, m.input, m.composeThread = false, nil, ""
		switch {
		case m.composeKind == kindThreadReply:
			m.decideThread(thread, state.VerdictOpen, text)
		case thread != "" && text != "":
			m.emit(inbox.Event{Kind: inbox.KindMessage, Text: "re thread " + thread + ": " + text})
		case text == "" && m.composeKind == inbox.KindAsk && m.anchorFile != "":
			m.emit(inbox.Event{Kind: inbox.KindExplain, File: m.anchorFile, Lines: m.anchorLines})
		case text == "":
		case m.composeKind == inbox.KindSkip && m.localSteps():
			m.moveStep("skip", "--reason", text)
		case m.rawMode():
			m.saveRaw(text)
		default:
			m.emit(inbox.Event{
				Kind: m.composeKind, Text: text, File: m.anchorFile, Lines: m.anchorLines,
				Comment: m.composeRef,
			})
		}
	case tea.KeyCtrlR:
		m.raw = !m.raw
	case tea.KeyTab:
		if m.rawMode() {
			i := slices.Index(state.Severities, m.severity())
			m.rawSeverity = state.Severities[(i+1)%len(state.Severities)]
		}
	case tea.KeyCtrlX:
		m.anchorFile, m.anchorLines, m.composeRef = "", "", 0
	case tea.KeyUp:
		m.inputLineMove(-1)
	case tea.KeyDown:
		m.inputLineMove(1)
	case tea.KeyLeft:
		m.inputPos = max(m.inputPos-1, 0)
	case tea.KeyRight:
		m.inputPos = min(m.inputPos+1, len(m.input))
	case tea.KeyCtrlA, tea.KeyHome:
		m.inputPos = 0
	case tea.KeyCtrlE, tea.KeyEnd:
		m.inputPos = len(m.input)
	case tea.KeyBackspace:
		if len(m.input) == 0 {
			m.anchorFile, m.anchorLines, m.composeRef = "", "", 0
		}
		if m.inputPos > 0 {
			m.input = append(m.input[:m.inputPos-1], m.input[m.inputPos:]...)
			m.inputPos--
		}
	case tea.KeyCtrlW:
		from := m.inputPos
		for from > 0 && m.input[from-1] == ' ' {
			from--
		}
		for from > 0 && m.input[from-1] != ' ' {
			from--
		}
		m.input = append(m.input[:from], m.input[m.inputPos:]...)
		m.inputPos = from
	case tea.KeyCtrlU:
		m.input, m.inputPos = nil, 0
	case tea.KeyRunes, tea.KeySpace:
		m.insert(msg.Runes)
	}
	return nil
}

func (m *model) inputLineMove(d int) {
	pos := min(m.inputPos, len(m.input))
	lineStart := func(i int) int {
		for i > 0 && m.input[i-1] != '\n' {
			i--
		}
		return i
	}
	lineEnd := func(i int) int {
		for i < len(m.input) && m.input[i] != '\n' {
			i++
		}
		return i
	}
	start := lineStart(pos)
	col := pos - start
	switch {
	case d < 0 && start > 0:
		m.inputPos = min(lineStart(start-1)+col, start-1)
	case d > 0 && lineEnd(pos) < len(m.input):
		next := lineEnd(pos) + 1
		m.inputPos = min(next+col, lineEnd(next))
	}
}

func (m *model) insert(rs []rune) {
	m.inputPos = min(m.inputPos, len(m.input))
	tail := append([]rune{}, m.input[m.inputPos:]...)
	m.input = append(append(m.input[:m.inputPos], rs...), tail...)
	m.inputPos += len(rs)
}

func (m *model) deleteComment() { m.deleteCommentID(m.current().Ref) }

func (m *model) deleteCommentID(ref int) {
	switch {
	case ref == 0:
		m.status = "put the cursor on one of your comments to delete it"
	case m.deleteArmed != ref:
		m.deleteArmed = ref
		m.status = fmt.Sprintf("press %s again to delete #%d", m.keys().key("delete-comment"), ref)
	default:
		m.deleteArmed = 0
		out, err := m.runGr("comment", "delete", fmt.Sprint(ref))
		out = strings.Join(strings.Split(strings.TrimSpace(out), "\n"), " · ")
		if err != nil {
			m.err = fmt.Errorf("%v: %s", err, out)
			return
		}
		m.emit(inbox.Event{Kind: inbox.KindComment, Text: out})
		m.status = out
		m.refreshPreview()
	}
}

func (m *model) emit(e inbox.Event) {
	if m.review == nil || m.step == nil && e.Kind != inbox.KindMessage {
		return
	}
	if e.Step == "" && m.step != nil {
		e.Step = m.step.ID
	}
	if err := m.send(e); err != nil {
		m.err = err
		return
	}
	m.visual = false
	m.status = "sent to agent: " + e.Kind
	if m.review != nil && m.store.Dir != "" {
		m.events, _ = inbox.All(m.store.ReviewDir(m.review.ID))
	}
	if m.step != nil && m.src != nil {
		m.rebuild(false)
	}
}

func (m *model) selection() (file, lines string, ok bool) {
	cur := m.current()
	if cur.File == "" || cur.Line == 0 {
		return "", "", false
	}
	if !m.visual {
		return cur.File, fmt.Sprint(cur.Line), true
	}
	lo, hi := min(m.anchor, m.cursor), max(m.anchor, m.cursor)
	first, last := 0, 0
	for i := lo; i <= hi && i < len(m.lines); i++ {
		it := m.lines[i]
		if it.File != cur.File || it.Line == 0 {
			continue
		}
		if first == 0 || it.Line < first {
			first = it.Line
		}
		last = max(last, it.Line)
	}
	if first == last {
		return cur.File, fmt.Sprint(first), true
	}
	return cur.File, fmt.Sprintf("%d-%d", first, last), true
}

func (m *model) selected(i int) bool {
	return m.visual && i >= min(m.anchor, m.cursor) && i <= max(m.anchor, m.cursor)
}

func (m *model) rawMode() bool {
	return m.raw && (m.composeKind == inbox.KindMessage || m.composeKind == inbox.KindEdit)
}

func (m *model) severity() state.Severity {
	return cmp.Or(m.rawSeverity, state.SeverityMinor)
}

func (m *model) saveRaw(text string) {
	args := []string{"comment", "edit", fmt.Sprint(m.composeRef), "--", text}
	if m.composeKind == inbox.KindMessage {
		if m.anchorFile == "" {
			m.err = errors.New("a raw comment needs a line: put the cursor on code")
			return
		}
		args = []string{
			"comment", "add", "--file", m.anchorFile, "--lines", m.anchorLines,
			"--severity", string(m.severity()),
		}
		if m.step != nil && !isExtra(m.step.ID) {
			args = append(args, "--step", m.step.ID)
		}
		args = append(args, "--", text)
	}
	out, err := m.runGr(args...)
	out = strings.Join(strings.Split(strings.TrimSpace(out), "\n"), " · ")
	if err != nil {
		m.err = fmt.Errorf("%v: %s", err, out)
		return
	}
	m.emit(inbox.Event{
		Kind: inbox.KindComment, File: m.anchorFile, Lines: m.anchorLines, Text: out,
	})
	m.status = "saved as written: " + out
}

func (m *model) noteDetails() {
	cur := m.current()
	if cur.Kind != RowNote || cur.Ref > 0 || !agentKinds[cur.NoteKind] {
		m.status = "put the cursor on one of the agent's notes"
		return
	}
	loc := lspLoc{Path: cur.File, Line: cur.Line}
	title := fmt.Sprintf("details · %s:%d", filepath.Base(cur.File), cur.Line)
	m.popup = &popup{kind: "detail", title: title, loc: loc}
	if m.refreshDetail() {
		return
	}
	m.popup.lines = []string{hotStyle.Render("the agent is writing the details…")}
	text := cur.Text
	for _, r := range m.rows {
		same := r.File == cur.File && r.Line == cur.Line && r.NoteKind == cur.NoteKind
		if r.Kind == RowNote && same {
			text = r.Text
			break
		}
	}
	m.emit(inbox.Event{
		Kind: inbox.KindDetail, File: cur.File, Lines: fmt.Sprint(cur.Line), Text: text,
	})
}

func (m *model) refreshDetail() bool {
	p := m.popup
	if p == nil || p.kind != "detail" || m.step == nil {
		return false
	}
	text, ok := m.step.Detail(p.loc.Path, p.loc.Line)
	if ok {
		width := min(max(m.mainWidth()-4, 20), detailWidth)
		p.refs = m.detailRefs(text)
		p.lines = append(markdownLines(expandTabs(text), width), m.refLines(p, width)...)
	}
	return ok
}

const (
	resumeAgent = "Continue the guided review: run gr wait."
	detailWidth = 96
)

func (m *model) runTmux(args ...string) error {
	if m.tmux != nil {
		return m.tmux(args...)
	}
	return exec.Command("tmux", args...).Run()
}

func (m *model) interrupt() {
	switch {
	case m.review == nil || m.agentWaiting || m.agentIdle:
		m.status = "the agent is not working now · " + m.keys().key("quit") + " quits"
	case m.returnPane == "":
		m.status = "no agent pane known: start the viewer with gr view --return $TMUX_PANE"
	default:
		if err := m.runTmux("send-keys", "-t", m.returnPane, "Escape"); err != nil {
			m.err = err
			return
		}
		m.interrupted = true
		m.startCompose(inbox.KindMessage)
		m.status = "agent interrupted: add to your question"
	}
}

func (m *model) resume() {
	if !m.interrupted {
		return
	}
	m.interrupted = false
	err := m.runTmux("send-keys", "-t", m.returnPane, "-l", resumeAgent)
	if err == nil {
		err = m.runTmux("send-keys", "-t", m.returnPane, "Enter")
	}
	if err != nil {
		m.err = err
	}
}

func (m *model) yank() {
	lo, hi := m.cursor, m.cursor
	if m.visual {
		lo, hi = min(m.anchor, m.cursor), max(m.anchor, m.cursor)
	}
	var out []string
	for i := lo; i <= hi && i < len(m.lines); i++ {
		switch l := m.lines[i]; l.Kind {
		case RowCode, RowAdded, RowRemoved:
			out = append(out, cmp.Or(l.Plain, ansi.Strip(l.Text)))
		case RowNote:
			out = append(out, l.Text)
		}
	}
	if len(out) == 0 {
		m.status = "nothing to copy here"
		return
	}
	if m.copyText(strings.Join(out, "\n")) {
		m.visual = false
		m.status = copiedStatus(len(out))
	}
}

func (m *model) copyText(text string) bool {
	copyText := m.clip
	if copyText == nil {
		copyText = clipboard
	}
	if err := copyText(text); err != nil {
		m.err = err
		return false
	}
	return true
}

func clipboard(text string) error {
	if os.Getenv("TMUX") != "" {
		cmd := exec.Command("tmux", "load-buffer", "-w", "-")
		cmd.Stdin = strings.NewReader(text)
		return cmd.Run()
	}
	encoded := base64.StdEncoding.EncodeToString([]byte(text))
	_, err := fmt.Fprintf(os.Stdout, "\x1b]52;c;%s\x07", encoded)
	return err
}

const inputRows = 5

type inputSeg struct {
	from, to int
	hard     bool
}

func wrapRunes(rs []rune, avail int) []inputSeg {
	avail = max(avail, 1)
	var segs []inputSeg
	from, x, space := 0, 0, -1
	for i, r := range rs {
		if r == '\n' {
			segs = append(segs, inputSeg{from, i, true})
			from, x, space = i+1, 0, -1
			continue
		}
		w := ansi.StringWidth(string(r))
		if x+w > avail && i > from {
			brk := i
			if space >= from {
				brk = space + 1
			}
			segs = append(segs, inputSeg{from, brk, false})
			from, space = brk, -1
			x = ansi.StringWidth(string(rs[from:i]))
		}
		if r == ' ' {
			space = i
		}
		x += w
	}
	return append(segs, inputSeg{from, len(rs), true})
}

func modifiedEnter(s string) (mod int, ok bool) {
	body, ok := strings.CutPrefix(s, "?CSI[")
	if !ok || !strings.HasSuffix(body, "]?") {
		return 0, false
	}
	body = strings.TrimSuffix(body, "]?")
	var seq []byte
	for _, f := range strings.Fields(body) {
		b, err := strconv.ParseUint(f, 10, 8)
		if err != nil {
			return 0, false
		}
		seq = append(seq, byte(b))
	}
	var code, param string
	switch text := string(seq); {
	case strings.HasPrefix(text, "27;") && strings.HasSuffix(text, "~"):
		param, code, _ = strings.Cut(strings.TrimSuffix(text[3:], "~"), ";")
	case strings.HasSuffix(text, "u"):
		code, param, _ = strings.Cut(strings.TrimSuffix(text, "u"), ";")
	}
	if code != "13" {
		return 0, false
	}
	n, err := strconv.Atoi(cmp.Or(param, "1"))
	if err != nil || n < 1 {
		return 0, false
	}
	return n - 1, true
}

func (m *model) inputLines(lead, hint string, width int) []string {
	width = max(width, 10)
	pos := min(m.inputPos, len(m.input))
	var lines []string
	indent, first := ansi.StringWidth(lead), lead
	if indent > width/2 {
		lines, indent, first = []string{lead}, 2, "  "
	}
	pad, cursorLine := strings.Repeat(" ", indent), 0
	for k, sg := range wrapRunes(m.input, width-indent-1) {
		text := string(m.input[sg.from:sg.to])
		if sg.from <= pos && (pos < sg.to || pos == sg.to && sg.hard) {
			text, cursorLine = withCursor(m.input[sg.from:sg.to], pos-sg.from), len(lines)
		}
		if k == 0 {
			lines = append(lines, first+text)
		} else {
			lines = append(lines, pad+text)
		}
	}
	rows := max(1, min(inputRows, (m.height-2)/3))
	start := min(max(len(lines)-rows, 0), cursorLine)
	lines = lines[start:min(start+rows, len(lines))]
	switch last := len(lines) - 1; {
	case hint == "":
	case ansi.StringWidth(lines[last])+ansi.StringWidth(hint) <= width:
		lines[last] += hint
	default:
		lines = append(lines, hint)
	}
	return lines
}
