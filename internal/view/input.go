package view

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/pltanton/guided-review/internal/inbox"
	"github.com/pltanton/guided-review/internal/state"
)

func (m *model) startCompose(kind string) {
	if m.review == nil || m.step == nil && kind != inbox.KindMessage {
		return
	}
	m.composing, m.composeKind, m.input, m.inputPos = true, kind, nil, 0
	m.composeRef, m.anchorFile, m.anchorLines = 0, "", ""
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
		m.composing, m.input = false, nil
	case tea.KeyEnter:
		text := strings.TrimSpace(string(m.input))
		m.composing, m.input = false, nil
		switch {
		case text == "" && m.composeKind == inbox.KindAsk && m.anchorFile != "":
			m.emit(inbox.Event{Kind: inbox.KindExplain, File: m.anchorFile, Lines: m.anchorLines})
		case text == "":
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

func (m *model) insert(rs []rune) {
	m.inputPos = min(m.inputPos, len(m.input))
	tail := append([]rune{}, m.input[m.inputPos:]...)
	m.input = append(append(m.input[:m.inputPos], rs...), tail...)
	m.inputPos += len(rs)
}

func (m *model) deleteComment() {
	ref := m.current().Ref
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
	agentNote := cur.Kind == RowNote && cur.Ref == 0 && cur.NoteKind != "mr" &&
		cur.NoteKind != "pending"
	if !agentNote {
		m.status = "put the cursor on one of the agent's notes"
		return
	}
	loc := lspLoc{Path: cur.File, Line: cur.Line}
	title := fmt.Sprintf("details · %s:%d", cur.File, cur.Line)
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
	m.emit(inbox.Event{Kind: inbox.KindDetail, File: cur.File, Lines: fmt.Sprint(cur.Line), Text: text})
}

func (m *model) refreshDetail() bool {
	p := m.popup
	if p == nil || p.kind != "detail" || m.step == nil {
		return false
	}
	text, ok := m.step.Detail(p.loc.Path, p.loc.Line)
	if ok {
		p.lines = markdownLines(expandTabs(text), max(m.mainWidth()-4, 20))
	}
	return ok
}
