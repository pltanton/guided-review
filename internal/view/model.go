package view

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/aplotnikov/guided-review/internal/gitx"
	"github.com/aplotnikov/guided-review/internal/state"
)

const defaultContext = 3

var (
	addStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	delStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	hotStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true)
	dimStyle    = lipgloss.NewStyle().Faint(true)
	fileStyle   = lipgloss.NewStyle().Bold(true).Underline(true)
	cursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true)
)

type reloadMsg struct{}

type editorDoneMsg struct{ err error }

type model struct {
	ctx     context.Context
	store   state.Store
	repo    gitx.Repo
	src     *gitSource
	review  *state.Review
	step    *state.Step
	rows    []Row
	cursor  int
	offset  int
	width   int
	height  int
	context int
	err     error
}

func newModel(ctx context.Context, store state.Store, repo gitx.Repo) *model {
	m := &model{ctx: ctx, store: store, repo: repo, context: defaultContext}
	m.reload()
	return m
}

func (m *model) reload() {
	r, err := m.store.LoadCurrent()
	if err != nil {
		m.review, m.step, m.rows, m.err = nil, nil, nil, err
		return
	}
	if m.src == nil || m.src.base != r.DiffBase() || m.src.head != r.HeadSHA {
		m.src = newGitSource(m.ctx, m.repo, r.DiffBase(), r.HeadSHA)
	}
	prev := ""
	if m.step != nil {
		prev = m.step.ID
	}
	m.review, m.err = r, nil
	m.step = r.Step(r.Current)
	if m.step == nil {
		m.rows = nil
		return
	}
	changed := m.step.ID != prev
	if changed {
		m.context, m.cursor, m.offset = defaultContext, 0, 0
	}
	m.rebuild(changed)
}

func (m *model) rebuild(jumpToHunk bool) {
	var keep Row
	if m.cursor < len(m.rows) {
		keep = m.rows[m.cursor]
	}
	rows, err := BuildRows(m.src, *m.step, m.context, nil)
	if err != nil {
		m.err = err
		return
	}
	m.rows = rows
	switch {
	case jumpToHunk:
		m.cursor = firstFocus(rows)
	case keep.File != "":
		for i, r := range rows {
			if r.File == keep.File && r.Line == keep.Line && r.Kind == keep.Kind {
				m.cursor = i
				break
			}
		}
	}
	m.clamp()
}

func firstFocus(rows []Row) int {
	for i, r := range rows {
		if r.HunkStart {
			return i
		}
	}
	for i, r := range rows {
		if r.Line > 0 {
			return i
		}
	}
	return 0
}

func (m *model) Init() tea.Cmd { return nil }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.clamp()
	case reloadMsg:
		m.reload()
	case editorDoneMsg:
		m.err = msg.err
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "j", "down":
			m.move(1)
		case "k", "up":
			m.move(-1)
		case "ctrl+d":
			m.move(m.bodyHeight() / 2)
		case "ctrl+u":
			m.move(-m.bodyHeight() / 2)
		case "g", "home":
			m.cursor = 0
			m.clamp()
		case "G", "end":
			m.cursor = len(m.rows) - 1
			m.clamp()
		case "]":
			if i := m.nextHunk(m.cursor); i >= 0 {
				m.cursor = i
				m.clamp()
			}
		case "[":
			if i := m.prevHunk(m.cursor); i >= 0 {
				m.cursor = i
				m.clamp()
			}
		case "tab":
			if m.step != nil {
				m.context += 10
				m.rebuild(false)
			}
		case "shift+tab":
			if m.step != nil {
				m.context = defaultContext
				m.rebuild(false)
			}
		case "e":
			return m, m.openEditor()
		}
	}
	return m, nil
}

func (m *model) move(d int) {
	m.cursor += d
	m.clamp()
}

func (m *model) clamp() {
	m.cursor = max(0, min(m.cursor, len(m.rows)-1))
	body := m.bodyHeight()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+body {
		m.offset = m.cursor - body + 1
	}
	m.offset = max(0, m.offset)
}

func (m *model) nextHunk(from int) int {
	for i := from + 1; i < len(m.rows); i++ {
		if m.rows[i].HunkStart {
			return i
		}
	}
	return -1
}

func (m *model) prevHunk(from int) int {
	for i := from - 1; i >= 0; i-- {
		if m.rows[i].HunkStart {
			return i
		}
	}
	return -1
}

func (m *model) header() []string {
	if m.step == nil {
		return nil
	}
	st := m.step
	title := fmt.Sprintf("%s %d/%d %s · %s", st.ID, m.review.StepIndex(st.ID)+1, len(m.review.Steps), st.Kind, st.Title)
	if st.Status != state.StatusPending {
		title += " [" + string(st.Status) + "]"
	}
	var cats []string
	for _, h := range st.Hotspots {
		cats = append(cats, h.Cat)
	}
	if len(cats) > 0 {
		title += "  " + hotStyle.Render("⚑ "+strings.Join(cats, " "))
	}
	lines := []string{lipgloss.NewStyle().Bold(true).Render(title)}
	if st.Note != "" {
		lines = append(lines, dimStyle.Render(st.Note))
	}
	for _, h := range st.Hotspots {
		lines = append(lines, hotStyle.Render("⚑ ")+h.Q)
	}
	if st.MayChange {
		lines = append(lines, delStyle.Render("may change after earlier comments"))
	}
	return append(lines, dimStyle.Render(strings.Repeat("─", max(m.width, 1))))
}

func (m *model) bodyHeight() int {
	return max(m.height-len(m.header())-1, 1)
}

func (m *model) View() string {
	if m.width == 0 {
		return ""
	}
	if m.review == nil {
		msg := "waiting for gr init…"
		if m.err != nil {
			msg = m.err.Error()
		}
		return dimStyle.Render(msg)
	}
	if m.step == nil {
		return dimStyle.Render(fmt.Sprintf("review %s: waiting for gr plan set…", m.review.ID))
	}
	var b strings.Builder
	for _, l := range m.header() {
		b.WriteString(ansi.Truncate(l, m.width, "…"))
		b.WriteByte('\n')
	}
	body := m.bodyHeight()
	for i := m.offset; i < m.offset+body; i++ {
		if i < len(m.rows) {
			b.WriteString(m.renderRow(i))
		}
		b.WriteByte('\n')
	}
	footer := dimStyle.Render("j/k move  ]/[ hunk  tab more context  e editor  q quit")
	if m.err != nil {
		footer = delStyle.Render(m.err.Error())
	}
	b.WriteString(ansi.Truncate(footer, m.width, "…"))
	return b.String()
}

func (m *model) renderRow(i int) string {
	r := m.rows[i]
	cursor := " "
	if i == m.cursor {
		cursor = cursorStyle.Render("▶")
	}
	var s string
	switch r.Kind {
	case RowFile:
		s = fileStyle.Render(r.Text)
	case RowGap:
		s = dimStyle.Render("      ⋯")
	default:
		marker, num, text := " ", fmt.Sprintf("%4d", r.Line), r.Text
		switch r.Kind {
		case RowAdded:
			marker = addStyle.Render("+")
		case RowRemoved:
			marker, num, text = delStyle.Render("-"), "    ", delStyle.Render(r.Text)
		}
		if r.Hotspot {
			marker = hotStyle.Render("⚑")
		}
		s = marker + dimStyle.Render(num+" │ ") + text
	}
	return ansi.Truncate(cursor+s, m.width, "")
}

func (m *model) openEditor() tea.Cmd {
	if m.cursor >= len(m.rows) || m.rows[m.cursor].File == "" {
		return nil
	}
	r := m.rows[m.cursor]
	dir := m.repo.Dir
	if m.review != nil && m.review.Worktree != "" {
		dir = m.review.Worktree
	}
	cmd := editorCmd(dir, r.File, max(r.Line, 1))
	return tea.ExecProcess(cmd, func(err error) tea.Msg { return editorDoneMsg{err} })
}

func editorCmd(dir, file string, line int) *exec.Cmd {
	editor := cmp.Or(os.Getenv("VISUAL"), os.Getenv("EDITOR"), "vi")
	target := fmt.Sprintf("+%d %s", line, shellQuote(file))
	if name := filepath.Base(strings.Fields(editor)[0]); name == "hx" || name == "helix" {
		target = shellQuote(fmt.Sprintf("%s:%d", file, line))
	}
	script := editor + " " + target
	var cmd *exec.Cmd
	if os.Getenv("TMUX") != "" {
		cmd = exec.Command("tmux", "display-popup", "-E", "-w", "90%", "-h", "90%", "-d", dir, script)
	} else {
		cmd = exec.Command("sh", "-c", script)
	}
	cmd.Dir = dir
	return cmd
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
