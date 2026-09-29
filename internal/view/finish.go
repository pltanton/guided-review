package view

import (
	"cmp"
	"fmt"
	"regexp"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const finishWidth = 110

var (
	boldMarkdown   = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	severityHead   = regexp.MustCompile("^(?:`[^`]+` )?\\*\\*(\\w+)\\*\\* ?")
	severityColors = map[string]string{"blocker": "1", "major": "3", "minor": "6", "nit": "8"}
)

func (m *model) finishColumn() (pad string, w int) {
	w = min(max(m.width-2, 20), finishWidth)
	return strings.Repeat(" ", max((m.width-w)/2, 0)), w
}

func (m *model) finishBody(w int) []string {
	var out []string
	for _, sec := range strings.Split("\n"+expandTabs(m.preview), "\n--- ")[1:] {
		head, body, _ := strings.Cut(sec, "\n")
		body = strings.TrimRight(body, "\n")
		if head == "summary" {
			rule := "── summary comment "
			rule += strings.Repeat("─", max(w-ansi.StringWidth(rule), 0))
			out = append(out, "", dimStyle.Render(rule))
			out = append(out, markdownLines(body, w)...)
			continue
		}
		severity := ""
		if mm := severityHead.FindStringSubmatch(body); mm != nil {
			severity, body = mm[1], strings.TrimPrefix(body, mm[0])
		}
		color := lipgloss.Color(severityColors[severity])
		badge := lipgloss.NewStyle().Foreground(color).Bold(true)
		title := badge.Render(fmt.Sprintf("● %-7s", severity))
		out = append(out, "", title+" "+fileStyle.Render(head))
		bar := lipgloss.NewStyle().Foreground(color).Render("  │ ")
		for _, l := range markdownLines(body, w-4) {
			out = append(out, bar+l)
		}
	}
	return out
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
	pad, w := m.finishColumn()
	r := m.review
	title := "finish"
	if r.MR != nil {
		title += fmt.Sprintf(" · !%d %s", r.MR.IID, r.MR.Title)
	}
	comments := strings.Count("\n"+m.preview, "\n--- ") - strings.Count(m.preview, "--- summary")
	facts := []string{fmt.Sprintf("%d comments to post", comments)}
	if p := r.Publish; p != nil {
		facts = append([]string{verdictStyle(p.Verdict)}, facts...)
		if p.Approve {
			facts = append(facts, "approve")
		}
	}
	if strings.Contains(m.preview, "--- summary") {
		facts = append(facts, "summary")
	}
	top := []string{
		"",
		boldStyle.Render(ansi.Truncate(title, w, "…")),
		dimStyle.Render(strings.Repeat("─", w)),
		strings.Join(facts, dimStyle.Render(" · ")),
	}
	body := m.finishBody(w)
	h := max(m.height-len(top)-2, 1)
	m.previewTop = max(0, min(m.previewTop, len(body)-h))
	shown := body[m.previewTop:min(len(body), m.previewTop+h)]
	lines := append(top, shown...)
	for len(lines) < m.height-1 {
		lines = append(lines, "")
	}
	k := m.keys().key("finish")
	hint := fmt.Sprintf("%s hand to the agent and close · esc back · j/k ctrl+d/u g/G wheel", k)
	if len(body) > h {
		hint += fmt.Sprintf(" · %d–%d of %d", m.previewTop+1, m.previewTop+len(shown), len(body))
	}
	lines = append(lines[:m.height-1], hotStyle.Render(hint))
	for i, l := range lines {
		lines[i] = pad + fit(l, w)
	}
	return strings.Join(lines, "\n")
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

func (m *model) handlePreviewKey(msg tea.KeyMsg) tea.Cmd {
	page := max(m.height/2, 1)
	switch msg.String() {
	case "P":
		return m.finish()
	case "esc", "q":
		m.preview = ""
	case "j", "down":
		m.previewTop++
	case "k", "up":
		m.previewTop = max(m.previewTop-1, 0)
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
