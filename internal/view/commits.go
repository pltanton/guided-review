package view

import (
	"cmp"
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/pltanton/guided-review/internal/state"
)

// tierUnchanged groups the files with nothing in the chosen commit range.
const tierUnchanged state.Tier = "unchanged"

// files is the tree's file list: the review's order, with a scope's unchanged files moved to
// their own folded group.
func (m *model) files() []state.File {
	files := m.review.FileOrder()
	if m.since == "" {
		return files
	}
	for i := range files {
		if files[i].Tier == state.TierCore && !m.scoped[files[i].Path] {
			files[i].Tier = tierUnchanged
		}
	}
	return files
}

// fileStat is the file's change count within the scope.
func (m *model) fileStat(f state.File) (added, deleted int) {
	if m.since == "" || m.src == nil {
		return f.Added, f.Deleted
	}
	fd, err := m.src.FileDiff(f.Path)
	if err != nil {
		return 0, 0
	}
	return fd.Stat()
}

func (m *model) scopeBase() string {
	return cmp.Or(m.since, m.review.DiffBase())
}

// rescope records which core files have changes in the current scope.
func (m *model) rescope() {
	m.scoped = nil
	if m.since == "" || m.src == nil {
		return
	}
	m.scoped = map[string]bool{}
	for _, f := range m.review.Files {
		if fd, err := m.src.FileDiff(f.Path); err == nil && len(fd.Hunks) > 0 {
			m.scoped[f.Path] = true
		}
	}
}

func (m *model) scopeLabel() string {
	if m.since == "" {
		return ""
	}
	n := "commit"
	if m.sinceN != 1 {
		n = "commits"
	}
	return fmt.Sprintf("since %d %s (%s)", m.sinceN, n, m.since[:min(7, len(m.since))])
}

// setSince narrows the diff to what changed after the commit; "" shows the whole change.
func (m *model) setSince(sha string, n int) {
	m.since, m.sinceN, m.src = sha, n, nil
	if m.tierOpen != nil {
		delete(m.tierOpen, tierUnchanged)
	}
	m.reload()
	m.status = "whole change"
	if sha != "" {
		m.status = m.scopeLabel()
	}
}

func (m *model) openCommits() {
	if m.review == nil {
		return
	}
	entries, err := m.repo.Log(m.ctx, m.review.BaseSHA, m.review.HeadSHA)
	if err != nil {
		m.err = err
		return
	}
	m.commits, m.commitCard, m.commitSel = entries, true, 0
	for i, c := range entries {
		if c.SHA == m.since {
			m.commitSel = i
		}
	}
}

// fromCommit narrows the diff to the n newest commits: the base is the one before them.
func (m *model) fromCommit(n int) {
	switch {
	case n <= 0 || n >= len(m.commits):
		m.setSince("", 0)
	default:
		m.setSince(m.commits[n].SHA, n)
	}
}

func (m *model) commitsModal(w int) modalContent {
	_, added, deleted := m.tierStat(state.TierCore)
	body := []string{cardItem(0, m.commitSel, "whole change  "+stat(added, deleted), w)}
	if m.review.Round > 1 && m.review.RoundBaseSHA != "" {
		body[0] += dimStyle.Render("  (this round: since " + m.review.RoundBaseSHA[:7] + ")")
	}
	for i, c := range m.commits {
		line := dimStyle.Render(c.SHA[:7]) + "  " + ansi.Truncate(c.Subject, max(w-32, 10), "…")
		gap := max(w-ansi.StringWidth(line)-16, 1)
		line += strings.Repeat(" ", gap) + stat(c.Added, c.Deleted)
		body = append(body, cardItem(i+1, m.commitSel, line, w))
	}
	return modalContent{"commits · newest first · enter shows the change from that commit on",
		"enter · esc close", body}
}

func (m *model) handleCommitsKey(msg tea.KeyMsg) tea.Cmd {
	switch cardNav(msg.String(), len(m.commits)+1, &m.commitSel) {
	case "esc":
		m.commitCard = false
	case "enter":
		m.commitCard = false
		m.fromCommit(m.commitSel)
	}
	return nil
}

// sinceCommand handles :since N (the last N commits), :since SHA and :since alone.
func (m *model) sinceCommand(arg string) {
	if m.review == nil {
		return
	}
	if arg == "" {
		m.setSince("", 0)
		return
	}
	entries, err := m.repo.Log(m.ctx, m.review.BaseSHA, m.review.HeadSHA)
	if err != nil {
		m.err = err
		return
	}
	m.commits = entries
	if n, err := strconv.Atoi(arg); err == nil {
		if n < 1 || n > len(entries) {
			m.status = fmt.Sprintf("the change has %d commits", len(entries))
			return
		}
		m.fromCommit(n)
		return
	}
	for i, c := range entries {
		if strings.HasPrefix(c.SHA, arg) {
			m.fromCommit(i + 1)
			return
		}
	}
	m.status = "no commit " + arg + " in the change"
}
