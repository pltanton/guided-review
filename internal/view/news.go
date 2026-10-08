package view

import (
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	guidedreview "github.com/pltanton/guided-review"
)

func (m *model) showNewsOnce() {
	if m.newsFile == "" || guidedreview.WhatsNew(guidedreview.Version) == "" {
		return
	}
	seen, err := os.ReadFile(m.newsFile)
	if err == nil && strings.TrimSpace(string(seen)) == guidedreview.Version {
		return
	}
	m.newsOpen, m.newsAll, m.newsTop = true, false, 0
}

func (m *model) openChangelog() {
	m.newsOpen, m.newsAll, m.newsTop = true, true, 0
}

func (m *model) newsModal(w, h int) modalContent {
	title, text := "what's new in v"+guidedreview.Version, guidedreview.WhatsNew(guidedreview.Version)
	if m.newsAll {
		title, text = "changelog", strings.TrimPrefix(guidedreview.Changelog, "# Changelog")
	}
	lines := markdownLines(strings.TrimSpace(text), min(w, detailWidth))
	m.newsTop = max(0, min(m.newsTop, len(lines)-h))
	hint := "j/k scroll · any key closes"
	if !m.newsAll {
		hint = ":changelog shows every version · any key closes"
	}
	return modalContent{title, hint, lines[m.newsTop:]}
}

func (m *model) handleNewsKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "j", "down":
		m.newsTop++
		return nil
	case "k", "up":
		m.newsTop = max(m.newsTop-1, 0)
		return nil
	}
	m.newsOpen = false
	if m.newsFile != "" && !m.newsAll {
		if err := os.MkdirAll(filepath.Dir(m.newsFile), 0o755); err == nil {
			_ = os.WriteFile(m.newsFile, []byte(guidedreview.Version+"\n"), 0o644)
		}
	}
	return nil
}
