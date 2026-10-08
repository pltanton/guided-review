package view

import (
	"cmp"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/pltanton/guided-review/internal/inbox"
	"github.com/pltanton/guided-review/internal/plan"
	"github.com/pltanton/guided-review/internal/state"
)

const finishWidth = 110

var (
	boldMarkdown  = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	severityHead  = regexp.MustCompile("^(?:`[^`]+` )?\\*\\*(\\w+)\\*\\* ?")
	severityTones map[string]tone
)

type finishCard struct {
	id, line int
}

func (m *model) finishBody(w int) (lines []string, cards []finishCard) {
	for _, sec := range strings.Split("\n"+expandTabs(state.WithoutMarker(m.preview)), "\n--- ")[1:] {
		head, body, _ := strings.Cut(sec, "\n")
		body = strings.TrimRight(body, "\n")
		if head == "summary" {
			rule := "── summary comment "
			rule += strings.Repeat("─", max(w-ansi.StringWidth(rule), 0))
			lines = append(lines, "", dimStyle.Render(rule))
			lines = append(lines, markdownLines(body, w)...)
			continue
		}
		if rest, ok := strings.CutPrefix(head, "thread "); ok {
			verdict := addStyle.Bold(true).Render("✔ resolve  ")
			if rest, ok = strings.CutPrefix(rest, "keep open "); ok {
				verdict = delStyle.Bold(true).Render("✖ keep open")
			} else {
				rest = strings.TrimPrefix(rest, "resolve ")
			}
			where, _, _ := strings.Cut(rest, " ")
			lines = append(lines, "", verdict+" "+fileStyle.Render(where)+dimStyle.Render("  thread"))
			for _, l := range markdownLines(body, w-4) {
				lines = append(lines, okTone.fg().Render("  │ ")+l)
			}
			continue
		}
		id := 0
		if ref, rest, ok := strings.Cut(head, " "); ok && strings.HasPrefix(ref, "#") {
			if n, err := strconv.Atoi(ref[1:]); err == nil {
				id, head = n, rest
			}
		}
		severity := ""
		if mm := severityHead.FindStringSubmatch(body); mm != nil {
			severity, body = mm[1], strings.TrimPrefix(body, mm[0])
		}
		t := severityTones[severity]
		badge := t.fg().Bold(true).Render(fmt.Sprintf("● %-7s", severity))
		title := badge + " " + fileStyle.Render(head)
		if id > 0 {
			title += dimStyle.Render(fmt.Sprintf("  #%d", id))
		}
		selected := len(cards) == m.previewSel && id > 0
		if selected {
			title = paint(fit(accentTone.fg().Render("▌")+title, w), cursorTone)
		}
		lines = append(lines, "")
		if id > 0 {
			cards = append(cards, finishCard{id: id, line: len(lines)})
		}
		lines = append(lines, title)
		bar := t.fg().Render("  │ ")
		for _, l := range markdownLines(body, w-4) {
			lines = append(lines, bar+l)
		}
	}
	return lines, cards
}

func markdownLines(text string, w int) []string {
	var out []string
	var fence string
	for _, l := range strings.Split(text, "\n") {
		switch {
		case strings.HasPrefix(l, "```") && fence == "":
			fence = cmp.Or(strings.TrimPrefix(l, "```"), "code")
			if strings.HasPrefix(fence, "suggestion") {
				fence = "suggestion"
			}
			out = append(out, dimStyle.Render("┄ "+fence))
			continue
		case strings.HasPrefix(l, "```"):
			fence = ""
			continue
		case fence == "suggestion":
			out = append(out, addStyle.Render("+ "+ansi.Truncate(l, w-2, "…")))
			continue
		case fence != "":
			out = append(out, "  "+ansi.Truncate(l, w-2, "…"))
			continue
		case strings.HasPrefix(l, "#"):
			l = boldStyle.Render(strings.TrimSpace(strings.TrimLeft(l, "#")))
		case strings.HasPrefix(l, "<sub>"):
			l = dimStyle.Render(strings.NewReplacer("<sub>", "", "</sub>", "").Replace(l))
		default:
			l = boldMarkdown.ReplaceAllString(l, boldStyle.Render("$1"))
		}
		out = append(out, strings.Split(ansi.Wrap(l, max(w, 10), ""), "\n")...)
	}
	return out
}

func (m *model) finishView() string {
	x, w, inner := m.cardFrame(finishWidth)
	r := m.review
	title := "finish"
	if r.MR != nil {
		title += " · " + r.MR.Label() + " " + r.MR.Title
	}
	threads := strings.Count("\n"+m.preview, "\n--- thread ")
	comments := strings.Count("\n"+m.preview, "\n--- ") - strings.Count(m.preview, "--- summary") -
		threads
	facts := []string{fmt.Sprintf("%d comments to post", comments)}
	if threads > 0 {
		facts = append(facts, fmt.Sprintf("%d thread replies", threads))
	}
	if p := r.Publish; p != nil {
		facts = append([]string{verdictStyle(p.Verdict)}, facts...)
		if p.Approve {
			facts = append(facts, "approve")
		}
	}
	if strings.Contains(m.preview, "--- summary") {
		facts = append(facts, "summary")
	}
	keys := m.keys().key("verdict") + " verdict"
	if r.MR != nil {
		keys += " · " + m.keys().key("approve") + " approve"
	}
	top := []string{
		strings.Join(facts, dimStyle.Render(" · ")) + hotStyle.Render("   "+keys), "",
	}
	body, cards := m.finishBody(w)
	h := max(inner-len(top)-1, 1)
	if m.previewFollow && m.previewSel < len(cards) {
		line := cards[m.previewSel].line
		m.previewTop = max(min(m.previewTop, line-1), line+3-h)
		m.previewFollow = false
	}
	m.previewTop = max(0, min(m.previewTop, len(body)-h))
	shown := body[m.previewTop:min(len(body), m.previewTop+h)]
	lines := append(top, shown...)
	for len(lines) < inner-1 {
		lines = append(lines, "")
	}
	km := m.keys()
	del := km.key("delete-comment")
	hint := fmt.Sprintf("%s hand to the agent · j/k comment · %s edit · %s%s delete · %s severity"+
		" · %s ask the agent · esc back", km.key("finish"), km.key("edit-comment"), del, del,
		km.key("severity"), km.key("message"))
	if len(body) > h {
		at := fmt.Sprintf("%d–%d of %d · ", m.previewTop+1, m.previewTop+len(shown), len(body))
		hint = at + hint
	}
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

func verdictStyle(v string) string {
	switch v {
	case "approve":
		return addStyle.Bold(true).Render("approve")
	case "blocked":
		return delStyle.Bold(true).Render("blocked")
	}
	return hotStyle.Render("changes requested")
}

func (m *model) previewSelected() int {
	_, cards := m.finishBody(max(m.width-2, 20))
	if m.previewSel < len(cards) {
		return cards[m.previewSel].id
	}
	return 0
}

func (m *model) handlePreviewKey(msg tea.KeyMsg) tea.Cmd {
	page := max(m.height/2, 1)
	_, cards := m.finishBody(max(m.width-2, 20))
	selected := m.previewSelected()
	km, k := m.keys(), msg.String()
	switch {
	case km.previewKey("finish", k):
		return m.finish()
	case km.previewKey("message", k) || km.previewKey("act", k):
		m.startCompose(inbox.KindMessage)
		m.anchorFile, m.anchorLines, m.composeRef = "", "", selected
		return nil
	case km.previewKey("edit-comment", k):
		m.editComment(selected)
		return nil
	case km.previewKey("delete-comment", k):
		m.deleteCommentID(selected)
		return nil
	}
	if i, ok := km.preview[k]; ok && km.actions[i].Group == finishGroup {
		return km.actions[i].run(m)
	}
	switch k {
	case "esc", "q":
		m.preview = ""
	case "j", "down":
		m.previewSel, m.previewFollow = min(m.previewSel+1, max(len(cards)-1, 0)), true
	case "k", "up":
		m.previewSel, m.previewFollow = max(m.previewSel-1, 0), true
	case "ctrl+d", "pgdown":
		m.previewTop += page
	case "ctrl+u", "pgup":
		m.previewTop = max(m.previewTop-page, 0)
	case "g", "home":
		m.previewTop = 0
	case "G", "end":
		m.previewTop = 1 << 20
	}
	return nil
}

var verdictCycle = []string{"approve", "changes", "blocked"}

func (m *model) publishPlan() *state.PublishPlan {
	if m.preview == "" || m.review == nil || m.review.Publish == nil || m.runGr == nil {
		m.status = "open the finish preview first (" + m.keys().key("finish") + ")"
		return nil
	}
	return m.review.Publish
}

func (m *model) cycleVerdict() {
	if p := m.publishPlan(); p != nil {
		m.autoVerdict = false
		i := slices.Index(verdictCycle, p.Verdict)
		next := verdictCycle[(i+1)%len(verdictCycle)]
		m.prepare(next, p.Approve && next == "approve")
	}
}

func (m *model) toggleApprove() {
	p := m.publishPlan()
	switch {
	case p == nil:
	case m.review.MR == nil:
		m.status = "approve needs a merge request"
	default:
		m.prepare(p.Verdict, !p.Approve)
	}
}

func (m *model) prepare(verdict string, approve bool) {
	plan := *m.review.Publish
	args := []string{"prepare", "--verdict", verdict, "--decisions", plan.Decisions}
	if approve {
		args = append(args, "--approve")
	}
	if out, err := m.runGr(args...); err != nil {
		m.err = fmt.Errorf("%v: %s", err, strings.TrimSpace(out))
		return
	}
	plan.Verdict, plan.Approve = verdict, approve
	m.review.Publish, m.err = &plan, nil
	m.status = "verdict: " + ansi.Strip(verdictStyle(verdict))
	if approve {
		m.status += " · approve"
	}
	m.refreshPreview()
}

func (m *model) editComment(id int) {
	for _, c := range m.review.Comments {
		if c.ID == id {
			m.composing, m.composeKind, m.composeRef = true, inbox.KindEdit, id
			m.anchorFile, m.anchorLines = "", ""
			m.input = []rune(c.Body)
			m.inputPos = len(m.input)
			return
		}
	}
	m.status = "select a comment first (j/k)"
}

func (m *model) cycleSeverity(id int) {
	for _, c := range m.review.Comments {
		if c.ID != id {
			continue
		}
		i := slices.Index(state.Severities, c.Severity)
		next := string(state.Severities[(i+1)%len(state.Severities)])
		args := []string{"comment", "edit", fmt.Sprint(id), "--severity", next, "--", c.Body}
		if out, err := m.runGr(args...); err != nil {
			m.err = fmt.Errorf("%v: %s", err, strings.TrimSpace(out))
			return
		}
		m.status = fmt.Sprintf("#%d is %s now", id, next)
		m.followVerdict(id, state.Severity(next))
		m.refreshPreview()
		return
	}
	m.status = "select a comment first (j/k)"
}

func (m *model) refreshPreview() {
	if m.preview == "" || m.runGr == nil {
		return
	}
	out, err := m.runGr("export", "--dry-run")
	if err != nil {
		m.err = fmt.Errorf("%v: %s", err, strings.TrimSpace(out))
		return
	}
	m.preview = out
}

func (m *model) askApprove(verdict string) bool {
	return m.review.MR != nil && verdict == plan.VerdictApprove
}

func (m *model) openFinish() {
	m.finishCard, m.finishSel, m.approvePick, m.autoVerdict = true, 0, 0, true
}

func (m *model) finishItems() []string {
	verdict, _ := plan.SuggestVerdict(m.review)
	if m.askApprove(verdict) {
		return []string{"approve the MR", "do not approve"}
	}
	return []string{"show the result"}
}

func (m *model) finishCardModal(w int) modalContent {
	verdict, why := plan.SuggestVerdict(m.review)
	body := []string{
		boldStyle.Render("verdict  ") + verdictStyle(verdict) + dimStyle.Render(" — "+why),
		dimStyle.Render("comments " + severityCounts(m.review)),
	}
	if d := plan.Decisions(m.review); d != "" {
		body = append(body, "")
		for _, l := range strings.Split(d, "\n") {
			body = append(body, strings.Split(ansi.Wrap(l, max(w, 20), ""), "\n")...)
		}
	}
	if waiting := m.threadsWaiting(); waiting != "" {
		body = append(body, "", waiting)
	}
	if m.askApprove(verdict) {
		body = append(body, "", hotStyle.Render("approve the MR when it is published? then the result"))
	}
	body = append(body, "")
	for i, it := range m.finishItems() {
		body = append(body, cardItem(i, m.finishSel, it, w))
	}
	if m.err != nil {
		body = append(body, "", delStyle.Render(m.err.Error()))
	}
	return modalContent{"finish", "enter · esc back to the review", body}
}

func severityCounts(r *state.Review) string {
	round := max(r.Round, 1)
	n := map[state.Severity]int{}
	for _, c := range r.Comments {
		if !c.Resolved && max(c.Round, 1) == round {
			n[c.Severity]++
		}
	}
	parts := make([]string, len(state.Severities))
	for i, s := range state.Severities {
		parts[i] = fmt.Sprintf("%d %s", n[s], s)
	}
	return strings.Join(parts, " · ")
}

func (m *model) handleFinishCardKey(msg tea.KeyMsg) tea.Cmd {
	items := m.finishItems()
	if i, ok := digitPick(msg.String(), len(items)); ok {
		m.finishSel = i
		return m.handleFinishCardKey(tea.KeyMsg{Type: tea.KeyEnter})
	}
	switch msg.String() {
	case "j", "down":
		m.finishSel = min(m.finishSel+1, len(items)-1)
	case "k", "up":
		m.finishSel = max(m.finishSel-1, 0)
	case "esc", "q":
		m.finishCard = false
	case m.keys().key("replies"):
		m.finishCard = false
		m.openThreads()
	case "enter":
		if len(items) == 2 {
			m.approvePick = m.finishSel + 1
		}
		return m.prepareFinish()
	}
	return nil
}

func (m *model) prepareFinish() tea.Cmd {
	verdict, _ := plan.SuggestVerdict(m.review)
	approve := m.askApprove(verdict) && m.approvePick == 1
	decisions := plan.Decisions(m.review)
	args := []string{"prepare", "--verdict", verdict, "--decisions", decisions}
	if approve {
		args = append(args, "--approve")
	}
	if len(plan.Gate(m.review)) > 0 {
		args = append(args, "--partial")
	}
	if out, err := m.runGr(args...); err != nil {
		m.err = fmt.Errorf("%v: %s", err, strings.TrimSpace(out))
		return nil
	}
	m.err = nil
	m.review.Publish = &state.PublishPlan{Verdict: verdict, Approve: approve, Decisions: decisions}
	m.finishCard = false
	return m.finish()
}

func (m *model) followVerdict(id int, sev state.Severity) {
	for i := range m.review.Comments {
		if m.review.Comments[i].ID == id {
			m.review.Comments[i].Severity = sev
		}
	}
	p := m.review.Publish
	if !m.autoVerdict || p == nil {
		return
	}
	if v, _ := plan.SuggestVerdict(m.review); v != p.Verdict {
		m.prepare(v, p.Approve && v == plan.VerdictApprove)
	}
}
