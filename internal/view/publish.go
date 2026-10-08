package view

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type publishCard struct {
	dir     string
	running bool
	done    bool
	tries   int
	sel     int
	out     string
	err     error
}

type publishedMsg struct {
	out string
	err error
}

const outputTail = 8

func (m *model) publishItems() []string {
	switch p := m.pub; {
	case p.running:
		return nil
	case p.done:
		return []string{"close the viewer"}
	case p.err != nil:
		return []string{"try again", "hand it to the agent"}
	}
	return []string{"publish to the MR now", "hand it to the agent"}
}

func (m *model) publishModal(w int) modalContent {
	p := m.pub
	verdict := "changes requested"
	if pp := m.review.Publish; pp != nil {
		verdict = pp.Verdict
		if pp.Approve {
			verdict += " · approve"
		}
	}
	body := []string{
		boldStyle.Render("goes to ") + m.review.MR.URL,
		dimStyle.Render(fmt.Sprintf("%s · verdict %s", severityCounts(m.review), verdict)),
		"",
	}
	switch {
	case p.running:
		body = append(body, hotStyle.Render(m.spin()+" publishing…"))
	case p.done:
		body = append(body, addStyle.Render("published"))
	case p.err != nil:
		body = append(body, delStyle.Render("publishing stopped: "+p.err.Error()))
	}
	if p.out != "" {
		lines := strings.Split(strings.TrimSpace(p.out), "\n")
		for _, l := range lines[max(len(lines)-outputTail, 0):] {
			body = append(body, dimStyle.Render(ansi.Truncate(l, w, "…")))
		}
	}
	body = append(body, "")
	for i, it := range m.publishItems() {
		body = append(body, cardItem(i, p.sel, it, w))
	}
	return modalContent{"publish", "enter · esc back", body}
}

func (m *model) handlePublishKey(msg tea.KeyMsg) tea.Cmd {
	p, items := m.pub, m.publishItems()
	if p.running {
		return nil
	}
	k := msg.String()
	if i, ok := digitPick(k, len(items)); ok {
		p.sel, k = i, "enter"
	}
	switch k {
	case "j", "down":
		p.sel = min(p.sel+1, len(items)-1)
	case "k", "up":
		p.sel = max(p.sel-1, 0)
	case "esc", "q":
		if !p.done {
			m.pub = nil
		}
	case "enter":
		switch {
		case p.done:
			return m.handOver("published " + p.dir)
		case p.sel == 1:
			dir := p.dir
			m.pub = nil
			return m.handOver(dir)
		}
		return m.startPublish()
	}
	return nil
}

func (m *model) startPublish() tea.Cmd {
	p := m.pub
	p.running, p.err, p.out = true, nil, ""
	p.tries++
	run, retry := m.runGr, p.tries > 1
	return func() tea.Msg {
		var log strings.Builder
		if retry {
			out, err := run("export")
			log.WriteString(out)
			if err != nil {
				return publishedMsg{out: log.String(), err: err}
			}
		}
		out, err := run("publish")
		log.WriteString(out)
		return publishedMsg{out: log.String(), err: err}
	}
}
