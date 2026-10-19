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
	kindSkip        = "skip"
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

func (m *model) openThreads() {
	if len(m.myThreads()) == 0 {
		m.status = "no open threads of yours on the MR"
		return
	}
	m.showStep(extraThreads)
}

func (m *model) threadNotes() []Note {
	r := m.review
	var out []Note
	for _, d := range m.myThreads() {
		t := r.ThreadState(d)
		add := func(kind, label, text string, dim bool) {
			n := Note{File: d.File, Line: d.Line, Kind: kind, Label: label, Text: text, Dim: dim,
				Thread: d.ID, Focus: true}
			if d.File == "" || d.OldLine {
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
			if i == 0 && d.OldLine {
				text = fmt.Sprintf("%s:%d (line removed) · %s", d.File, d.Line, text)
			} else if i == 0 && d.File == "" {
				text = "general · " + text
			}
			if n.Author == r.MR.Me {
				add("comment", "YOU", text, false)
			} else {
				add("mr", "@"+n.Author, text, false)
			}
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
	return "NO REPLY", "the author has not answered yet · enter to resolve anyway"
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
}

func (m *model) threadBadge(d state.Discussion) (label string, resolved bool) {
	r := m.review
	if r.MR == nil || d.Author != r.MR.Me {
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
	if !m.mine(d) {
		return []string{"discuss it with the agent"}
	}
	t := m.review.ThreadState(d)
	items := []string{"resolve", "keep open and reply to the author"}
	if t.Proposed != "" {
		items = append(items, "as the agent proposes: "+verdictWord(t.Proposed))
	}
	if t.Decided() {
		items = append(items, "undo the decision")
	}
	return items
}

func (m *model) threadCardModal(w int) modalContent {
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
	if m.mine(d) {
		label, _ := threadDecision(m.review, d, t)
		body = append(body, "", "now: "+label)
	}
	body = append(body, "")
	for i, it := range m.threadCardItems(d) {
		body = append(body, cardItem(i, m.threadCardSel, it, w))
	}
	title := fmt.Sprintf("thread · %s:%d · @%s", d.File, d.Line, d.Author)
	return modalContent{title, "enter · esc back", body}
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
	case item == "keep open and reply to the author":
		reply := cmp.Or(t.Reply, t.ProposedReply)
		m.composing, m.composeKind, m.composeThread = true, kindThreadReply, d.ID
		m.input, m.inputPos = []rune(reply), len([]rune(reply))
		m.placeInline()
	case strings.HasPrefix(item, "as the agent proposes"):
		m.decideThread(d.ID, t.Proposed, t.ProposedReply)
	case item == "undo the decision":
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
	return hotStyle.Render(fmt.Sprintf("%d answered %s for your decision · press %s",
		n, what, m.keys().key("replies")))
}
