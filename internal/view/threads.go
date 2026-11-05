package view

import (
	"cmp"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/pltanton/guided-review/internal/state"
)

const (
	kindThreadReply = "thread-reply"
	kindReplyAsk    = "reply-ask"
	itemReply       = "reply to the author"
	itemAskReply    = "ask the agent for a reply, with directions"
	itemAgentReply  = "send the reply the agent suggests"
	itemDiscuss     = "discuss it with the agent"
)

func (m *model) myThreads() []state.Discussion {
	if m.review == nil {
		return nil
	}
	return m.review.MyThreads()
}

func (m *model) pendingThreads() int {
	n := 0
	for _, d := range m.myThreads() {
		if m.review.Answered(d) && !m.review.ThreadState(d).Decided() {
			n++
		}
	}
	return n
}

func (m *model) assessing(d state.Discussion) bool {
	t := m.review.ThreadState(d)
	return m.review.Answered(d) && !t.Decided() && t.Proposed == "" &&
		!m.agentWaiting && !m.agentIdle
}

func (m *model) threadsIntro() Note {
	k, answered := m.keys(), 0
	for _, d := range m.myThreads() {
		if m.review.Answered(d) {
			answered++
		}
	}
	text := fmt.Sprintf("The author answered %d of your %d open threads. They sit on their lines "+
		"(%s/%s walk the notes): read the reply and the agent's take, then %s — resolve, or keep "+
		"it open with a reply. Nothing reaches the MR before you publish.",
		answered, len(m.myThreads()), k.key("next-note"), k.key("prev-note"), k.key("act"))
	return Note{Top: true, Kind: "note", Label: fmt.Sprintf("ROUND %d", m.review.Round), Text: text}
}

// lineGone is true when the thread's line is past the end of the file as it is now.
func (m *model) lineGone(d state.Discussion) bool {
	if m.src == nil || d.File == "" {
		return false
	}
	lines, err := m.src.Lines(d.File)
	return err == nil && d.Line > len(lines)
}

func (m *model) threadNotes() []Note {
	r := m.review
	var out []Note
	for _, d := range m.myThreads() {
		t := r.ThreadState(d)
		gone := m.lineGone(d)
		add := func(kind, label, text string, dim bool) {
			n := Note{File: d.File, Line: d.Line, Kind: kind, Label: label, Text: text, Dim: dim,
				Thread: d.ID, Focus: true}
			if d.File == "" || d.OldLine || r.File(d.File) == nil || gone {
				n.File, n.Line, n.Top = "", 0, true
			}
			out = append(out, n)
		}
		notes := d.Notes
		if len(notes) == 0 {
			notes = []state.Note{{Author: d.Author, Body: d.Body}}
		}
		for i, n := range notes {
			text := chatSpan.ReplaceAllString(strings.TrimSpace(n.Body), "$1$2")
			switch {
			case i == 0 && d.OldLine:
				text = fmt.Sprintf("%s:%d (line removed) · %s", d.File, d.Line, text)
			case i == 0 && d.File == "":
				text = "general · " + text
			case i == 0 && r.File(d.File) == nil:
				text = fmt.Sprintf("%s:%d (not in this round) · %s", d.File, d.Line, text)
			case i == 0 && gone:
				text = fmt.Sprintf("%s:%d (the line is gone) · %s", d.File, d.Line, text)
			}
			if n.Author == r.MR.Me {
				add("comment", "YOU", text, false)
			} else {
				add("mr", "@"+n.Author, text, false)
			}
		}
		if m.assessing(d) {
			add("pending", "", "agent is checking the reply against the code…", false)
		}
		if t.Assessment != "" {
			text := t.Assessment
			if t.ProposedReply != "" && !t.Decided() {
				text += "\nreply it suggests: " + t.ProposedReply
			}
			add("note", "AGENT · "+verdictWord(t.Proposed)+"?", text, t.Decided())
		}
		label, text := threadDecision(r, d, t)
		add("hotspot", label, text, t.Decided())
	}
	return out
}

func threadDecision(r *state.Review, d state.Discussion, t state.Thread) (label, text string) {
	switch {
	case t.Decided() && t.Verdict == state.VerdictResolve:
		return "✔ RESOLVE", "resolved on publishing · enter to change"
	case t.Decided() && t.Reply != "":
		return "✖ KEEP OPEN", "your reply, sent on publishing: " + t.Reply
	case t.Decided():
		return "✖ KEEP OPEN", "stays open, no reply · enter to change"
	case r.Answered(d):
		return "DECIDE", "enter: resolve, or keep open and reply to the author"
	}
	return "NO ANSWER YET", "the author has not replied · enter to resolve anyway"
}

func verdictWord(v string) string {
	if v == state.VerdictOpen {
		return "keep open"
	}
	return v
}

func (m *model) decideThread(id, verdict, reply string) {
	err := m.store.UpdateCurrent(func(r *state.Review) error {
		return r.Decide(id, verdict, reply, time.Now())
	})
	if err != nil {
		m.err = err
		return
	}
	m.reload()
	m.status = "thread " + verdictWord(verdict)
	if verdict == state.VerdictNone {
		return
	}
	if n := m.pendingThreads(); n > 0 {
		m.status += fmt.Sprintf(" · %d more to decide, %s walks the notes", n, m.keys().key("next-note"))
	} else {
		m.status = "all answered threads decided"
	}
}

func (m *model) threadBadge(d state.Discussion) (label string, resolved bool) {
	r := m.review
	if r.MR == nil || d.Author != r.MR.Me {
		if t := r.ThreadState(d); t.Decided() && t.Reply != "" {
			return "@" + d.Author + " ↩ your reply", false
		}
		return "@" + d.Author, false
	}
	switch t := r.ThreadState(d); {
	case t.Decided() && t.Verdict == state.VerdictResolve:
		return "✓ RESOLVE", true
	case t.Decided() && t.Verdict == state.VerdictOpen:
		return "↩ OPEN", false
	case len(d.Notes) > 0:
		return fmt.Sprintf("YOU ↩%d", len(d.Notes)), false
	}
	return "YOU", false
}

func (m *model) mine(d state.Discussion) bool {
	return m.review.MR != nil && d.Author == m.review.MR.Me && d.Resolvable && !d.Resolved
}

func (m *model) threadCardItems(d state.Discussion) []string {
	t := m.review.ThreadState(d)
	if !m.mine(d) {
		items := []string{itemReply, itemAskReply}
		if t.ProposedReply != "" {
			items = append(items, itemAgentReply)
		}
		if t.Decided() {
			items = append(items, "undo the reply")
		}
		return append(items, itemDiscuss)
	}
	items := []string{"resolve", "keep open and reply to the author", itemAskReply}
	if t.Proposed != "" {
		items = append(items, "as the agent proposes: "+verdictWord(t.Proposed))
	}
	if t.Decided() {
		items = append(items, "undo the decision")
	}
	return items
}

func (m *model) threadCardModal(w, h int) modalContent {
	dp := m.review.Discussion(m.threadCard)
	if dp == nil {
		m.threadCard = ""
		return modalContent{"thread", "esc", nil}
	}
	d := *dp
	t := m.review.ThreadState(d)
	var body []string
	say := func(who, text string) {
		lead := youStyle.Render(who)
		if m.review.MR == nil || who != m.review.MR.Me {
			lead = agentStyle.Render(who)
		}
		for i, l := range strings.Split(ansi.Wrap(text, max(w-4, 20), ""), "\n") {
			if i == 0 {
				body = append(body, lead+dimStyle.Render(" │ ")+l)
			} else {
				body = append(body, strings.Repeat(" ", ansi.StringWidth(who))+dimStyle.Render(" │ ")+l)
			}
		}
	}
	say(d.Author, d.Body)
	for _, n := range d.Notes {
		say(n.Author, n.Body)
	}
	if t.Proposed != "" {
		body = append(body, "", agentTone.fg().Render("agent: "+verdictWord(t.Proposed)+" — ")+t.Assessment)
		if t.ProposedReply != "" {
			body = append(body, dimStyle.Render("reply it suggests: ")+t.ProposedReply)
		}
	}
	if m.mine(d) || t.Decided() {
		label, _ := threadDecision(m.review, d, t)
		body = append(body, "", "now: "+label)
	}
	items := m.threadCardItems(d)
	// the conversation scrolls (j/k) in the room the choices leave
	hint := "enter · esc back"
	if room := h - len(items) - 2; len(body) > room && room > 3 {
		m.threadCardTop = max(0, min(m.threadCardTop, len(body)-room))
		above, below := m.threadCardTop, len(body)-room-m.threadCardTop
		body = body[m.threadCardTop : m.threadCardTop+room]
		if above > 0 {
			body[0] = dimStyle.Render(fmt.Sprintf("↑ %d more", above))
		}
		if below > 0 {
			body[len(body)-1] = dimStyle.Render(fmt.Sprintf("↓ %d more", below))
		}
		hint = "j/k scroll · 1-9 or ↑↓ enter pick · esc back"
	} else {
		m.threadCardTop = 0
	}
	body = append(body, "")
	for i, it := range items {
		body = append(body, cardItem(i, m.threadCardSel, it, w))
	}
	title := fmt.Sprintf("thread · %s:%d · @%s", d.File, d.Line, d.Author)
	return modalContent{title, hint, body}
}

func (m *model) handleThreadCardKey(msg tea.KeyMsg) tea.Cmd {
	dp := m.review.Discussion(m.threadCard)
	if dp == nil {
		m.threadCard = ""
		return nil
	}
	d := *dp
	items := m.threadCardItems(d)
	m.threadCardSel = max(0, min(m.threadCardSel, len(items)-1))
	switch k := msg.String(); k {
	case "j", "ctrl+e":
		m.threadCardTop++
		return nil
	case "k", "ctrl+y":
		m.threadCardTop = max(m.threadCardTop-1, 0)
		return nil
	}
	switch cardNav(msg.String(), len(items), &m.threadCardSel) {
	case "esc":
		m.threadCard = ""
	case "enter":
		m.threadCard = ""
		m.threadAction(d, items[m.threadCardSel])
	}
	return nil
}

func (m *model) threadAction(d state.Discussion, item string) {
	t := m.review.ThreadState(d)
	switch {
	case item == "resolve":
		m.decideThread(d.ID, state.VerdictResolve, "")
	case item == "keep open and reply to the author" || item == itemReply:
		reply := cmp.Or(t.Reply, t.ProposedReply)
		m.composing, m.composeKind, m.composeThread = true, kindThreadReply, d.ID
		m.input, m.inputPos = []rune(reply), len([]rune(reply))
		m.placeInline()
	case item == itemAskReply:
		m.composing, m.composeKind, m.composeThread = true, kindReplyAsk, d.ID
		m.input, m.inputPos = nil, 0
		m.placeInline()
	case strings.HasPrefix(item, "as the agent proposes"):
		m.decideThread(d.ID, t.Proposed, t.ProposedReply)
	case item == itemAgentReply:
		m.decideThread(d.ID, state.VerdictOpen, t.ProposedReply)
	case item == "undo the decision" || item == "undo the reply":
		m.decideThread(d.ID, state.VerdictNone, "")
	default:
		m.startChat(true)
		m.composeThread = d.ID
	}
}

func (m *model) threadsWaiting() string {
	n := m.pendingThreads()
	if n == 0 {
		return ""
	}
	what := "threads of yours wait"
	if n == 1 {
		what = "thread of yours waits"
	}
	return hotStyle.Render(fmt.Sprintf("%d answered %s for your decision · %s walks the notes",
		n, what, m.keys().key("next-note")))
}
