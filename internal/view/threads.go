package view

import (
	"cmp"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/pltanton/guided-review/internal/inbox"
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
	m.threads, m.threadSel, m.threadTop, m.threadFollow = true, 0, 0, true
}

func (m *model) selectedThread() (state.Discussion, state.Thread, bool) {
	ds := m.myThreads()
	if len(ds) == 0 {
		return state.Discussion{}, state.Thread{}, false
	}
	d := ds[min(m.threadSel, len(ds)-1)]
	return d, m.review.ThreadState(d), true
}

func (m *model) threadBody(w int) (lines []string, starts []int) {
	r := m.review
	for i, d := range m.myThreads() {
		t := r.ThreadState(d)
		sev := state.ThreadSeverity(d)
		tn := severityTones[string(sev)]
		where := "general"
		if d.File != "" {
			where = fmt.Sprintf("%s:%d", d.File, d.Line)
		}
		title := tn.fg().Bold(true).Render(fmt.Sprintf("● %-7s", sev)) + " " + fileStyle.Render(where)
		if c := r.CommentFor(d); c != nil {
			title += dimStyle.Render(fmt.Sprintf("  #%d", c.ID))
		}
		status := threadStatus(r, d, t)
		gap := max(w-ansi.StringWidth(title)-ansi.StringWidth(status)-1, 1)
		title += strings.Repeat(" ", gap) + status
		if i == m.threadSel {
			title = paint(fit(accentTone.fg().Render("▌")+title, w), cursorTone)
		}
		lines = append(lines, "")
		starts = append(starts, len(lines))
		lines = append(lines, title)
		bar := tn.fg().Render("  │ ")
		if d.File != "" && !d.OldLine {
			target := d.Line - 1
			for _, l := range codeLines(m.peek(d.File), max(target-1, 0), target, 3) {
				lines = append(lines, bar+ansi.Truncate(l, w-4, ""))
			}
			lines = append(lines, bar)
		}
		say := func(who string, st tone, text string) {
			label := st.fg().Bold(true).Render(fmt.Sprintf("%-9s", who))
			for j, l := range markdownLines(strings.TrimSpace(text), w-4-9) {
				if j > 0 {
					label = strings.Repeat(" ", 9)
				}
				lines = append(lines, bar+label+l)
			}
		}
		for _, n := range d.Notes {
			if n.Author == r.MR.Me {
				say("you", youTone, n.Body)
			} else {
				say("@"+n.Author, textTone, n.Body)
			}
		}
		if t.Assessment != "" {
			say("agent", agentTone, verdictWord(t.Proposed)+": "+t.Assessment)
			if t.ProposedReply != "" && !t.Decided() {
				say("", agentTone, "reply: "+t.ProposedReply)
			}
		}
		if t.Reply != "" {
			say("reply ›", okTone, t.Reply)
		}
	}
	return lines, starts
}

func threadStatus(r *state.Review, d state.Discussion, t state.Thread) string {
	switch {
	case t.Decided() && t.Verdict == state.VerdictResolve:
		return addStyle.Bold(true).Render("✔ resolve")
	case t.Decided() && t.Verdict == state.VerdictOpen:
		return delStyle.Bold(true).Render("✖ keep open")
	case t.Proposed != "":
		return agentTone.fg().Render("agent: " + verdictWord(t.Proposed) + "?")
	case r.Answered(d):
		return hotStyle.Render("answered")
	}
	return dimStyle.Render("no reply")
}

func verdictWord(v string) string {
	if v == state.VerdictOpen {
		return "keep open"
	}
	return v
}

func (m *model) threadsView() string {
	x, w, inner := m.cardFrame(finishWidth)
	r := m.review
	title := "replies"
	if r.MR != nil {
		title += " · " + r.MR.Label() + " " + r.MR.Title
	}
	ds := m.myThreads()
	answered, decided := 0, 0
	for _, d := range ds {
		if r.Answered(d) {
			answered++
		}
		if r.ThreadState(d).Decided() {
			decided++
		}
	}
	facts := fmt.Sprintf("%d open threads of yours · %d answered · %d decided",
		len(ds), answered, decided)
	top := []string{dimStyle.Render(facts), ""}
	body, starts := m.threadBody(w)
	h := max(inner-len(top)-1, 1)
	if m.threadFollow && m.threadSel < len(starts) {
		line := starts[m.threadSel]
		end := len(body)
		if m.threadSel+1 < len(starts) {
			end = starts[m.threadSel+1] - 1
		}
		m.threadTop = min(max(m.threadTop, end-h), line-1)
		m.threadFollow = false
	}
	m.threadTop = max(0, min(m.threadTop, len(body)-h))
	lines := append(top, body[m.threadTop:min(len(body), m.threadTop+h)]...)
	for len(lines) < inner-1 {
		lines = append(lines, "")
	}
	hint := "j/k thread · r resolve · o keep open + reply · a take the agent's · u undo" +
		" · c ask the agent · esc back"
	bottom := []string{hotStyle.Render(hint)}
	switch {
	case m.composing:
		bottom = m.promptLines(w)
	case m.err != nil:
		bottom = []string{delStyle.Render(m.err.Error())}
	case m.status != "":
		bottom = []string{dimStyle.Render(m.status)}
	}
	lines = append(lines[:max(inner-len(bottom), 0)], bottom...)
	return m.card(x, w, title, lines)
}

func (m *model) handleThreadsKey(msg tea.KeyMsg) tea.Cmd {
	n := len(m.myThreads())
	d, t, ok := m.selectedThread()
	switch msg.String() {
	case "esc", "q", m.keys().key("replies"):
		m.threads = false
	case "j", "down":
		m.threadSel, m.threadFollow = min(m.threadSel+1, max(n-1, 0)), true
	case "k", "up":
		m.threadSel, m.threadFollow = max(m.threadSel-1, 0), true
	case "ctrl+d", "pgdown":
		m.threadTop += max(m.height/2, 1)
	case "ctrl+u", "pgup":
		m.threadTop = max(m.threadTop-max(m.height/2, 1), 0)
	case "r":
		if ok {
			m.decideThread(d.ID, state.VerdictResolve, "")
		}
	case "o":
		if ok {
			reply := t.Reply
			if reply == "" {
				reply = t.ProposedReply
			}
			m.composing, m.composeKind, m.composeThread = true, kindThreadReply, d.ID
			m.input, m.inputPos = []rune(reply), len([]rune(reply))
		}
	case "a":
		switch {
		case !ok:
		case t.Proposed == "":
			m.status = "the agent has not assessed this thread yet"
		default:
			m.decideThread(d.ID, t.Proposed, t.ProposedReply)
		}
	case "u":
		if ok {
			m.decideThread(d.ID, state.VerdictNone, "")
		}
	case "c", "enter":
		if ok {
			m.startCompose(inbox.KindMessage)
			m.anchorFile, m.anchorLines, m.composeRef = "", "", 0
			m.composeThread = d.ID
		}
	}
	return nil
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

func (m *model) cardThread() (state.Discussion, bool) {
	for _, d := range m.review.Discussions {
		if d.ID == m.threadCard {
			return d, true
		}
	}
	return state.Discussion{}, false
}

func (m *model) mine(d state.Discussion) bool {
	return m.review.MR != nil && d.Author == m.review.MR.Me && d.Resolvable && !d.Resolved
}

func (m *model) threadCardItems(d state.Discussion) []string {
	if !m.mine(d) {
		return []string{"discuss it with the agent"}
	}
	t := m.review.ThreadState(d)
	items := []string{"resolve", "reply and keep open"}
	if t.Proposed != "" {
		items = append(items, "take the agent's: "+verdictWord(t.Proposed))
	}
	if t.Decided() {
		items = append(items, "undo the decision")
	}
	return items
}

func (m *model) threadCardModal(w int) modalContent {
	d, ok := m.cardThread()
	if !ok {
		m.threadCard = ""
		return modalContent{"thread", "esc", nil}
	}
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
		body = append(body, "", "now: "+threadStatus(m.review, d, t))
	}
	body = append(body, "")
	for i, it := range m.threadCardItems(d) {
		body = append(body, cardItem(i, m.threadCardSel, it, w))
	}
	title := fmt.Sprintf("thread · %s:%d · @%s", d.File, d.Line, d.Author)
	return modalContent{title, "enter · esc back", body}
}

func (m *model) handleThreadCardKey(msg tea.KeyMsg) tea.Cmd {
	d, ok := m.cardThread()
	if !ok {
		m.threadCard = ""
		return nil
	}
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
	case item == "reply and keep open":
		reply := cmp.Or(t.Reply, t.ProposedReply)
		m.composing, m.composeKind, m.composeThread = true, kindThreadReply, d.ID
		m.input, m.inputPos = []rune(reply), len([]rune(reply))
	case strings.HasPrefix(item, "take the agent's"):
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
