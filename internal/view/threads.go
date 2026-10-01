package view

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/pltanton/guided-review/internal/inbox"
	"github.com/pltanton/guided-review/internal/state"
)

const kindThreadReply = "thread-reply"

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
	pad, w := m.finishColumn()
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
	top := []string{
		"",
		boldStyle.Render(ansi.Truncate(title, w, "…")),
		dimStyle.Render(strings.Repeat("─", w)),
		dimStyle.Render(facts),
	}
	body, starts := m.threadBody(w)
	h := max(m.height-len(top)-2, 1)
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
	for len(lines) < m.height-1 {
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
	lines = append(lines[:max(m.height-len(bottom), 0)], bottom...)
	for i, l := range lines {
		lines[i] = pad + fit(l, w)
	}
	return strings.Join(lines, "\n")
}

func (m *model) handleThreadsKey(msg tea.KeyMsg) tea.Cmd {
	n := len(m.myThreads())
	d, t, ok := m.selectedThread()
	switch msg.String() {
	case "esc", "q", "R":
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
