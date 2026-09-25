package view

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/aplotnikov/guided-review/internal/gitx"
	"github.com/aplotnikov/guided-review/internal/inbox"
	"github.com/aplotnikov/guided-review/internal/state"
)

const (
	defaultContext = 3
	minSplitWidth  = 80
	minPlanWidth   = 90
	maxPlanWidth   = 32
	messageLines   = 5
)

type reloadMsg struct{}

type tickMsg struct{}

const tickEvery = 150 * time.Millisecond

func tick() tea.Cmd {
	return tea.Tick(tickEvery, func(time.Time) tea.Msg { return tickMsg{} })
}

type editorDoneMsg struct{ err error }

type item struct {
	File      string
	Line      int
	HunkStart bool
	Note      bool
	FileHead  bool
}

type model struct {
	ctx    context.Context
	store  state.Store
	repo   gitx.Repo
	src    *gitSource
	review *state.Review
	step   *state.Step
	rows   []Row
	disp   []Row
	split  []SplitRow
	list   []item
	events []inbox.Event

	cursor, offset int
	width, height  int
	context        int

	splitView, showPlan, mouse bool
	visual, dragging           bool
	anchor                     int

	focusFiles bool
	fileCursor int

	composing   bool
	composeKind string
	input       []rune

	send   func(inbox.Event) error
	status string
	err    error

	now          func() time.Time
	agentWaiting bool
	agentSince   time.Time
	lastWait     time.Time
	frame        int
}

func newModel(ctx context.Context, store state.Store, repo gitx.Repo) *model {
	m := &model{ctx: ctx, store: store, repo: repo, context: defaultContext, showPlan: true, mouse: true}
	m.send = func(e inbox.Event) error {
		if m.review == nil {
			return state.ErrNoReview
		}
		return inbox.Append(m.store.ReviewDir(m.review.ID), e)
	}
	m.reload()
	return m
}

func (m *model) reload() {
	r, err := m.store.LoadCurrent()
	if err != nil {
		m.review, m.step, m.rows, m.split, m.list, m.err = nil, nil, nil, nil, nil, err
		return
	}
	if m.src == nil || m.src.base != r.DiffBase() || m.src.head != r.HeadSHA {
		m.src = newGitSource(m.ctx, m.repo, r.DiffBase(), r.HeadSHA)
	}
	m.events, _ = inbox.All(m.store.ReviewDir(r.ID))
	m.lastWait, _ = inbox.LastWait(m.store.ReviewDir(r.ID))
	prev := ""
	if m.step != nil {
		prev = m.step.ID
	}
	m.review, m.err = r, nil
	m.step = r.Step(r.Current)
	if m.step == nil {
		m.rows, m.split, m.list = nil, nil, nil
		return
	}
	changed := m.step.ID != prev
	if changed {
		m.context, m.cursor, m.offset, m.visual = defaultContext, 0, 0, false
	}
	m.rebuild(changed)
}

func (m *model) rebuild(jumpToHunk bool) {
	rows, err := BuildRows(m.src, *m.step, m.context, m.notes())
	if err != nil {
		m.err = err
		return
	}
	keep := m.current()
	m.rows = rows
	m.relist()
	switch {
	case jumpToHunk:
		m.cursor = firstFocus(m.list)
	case keep.File != "":
		m.focus(keep)
	}
	m.clamp()
}

func (m *model) notes() []Note {
	var out []Note
	for _, a := range m.step.Annotations {
		out = append(out, Note{File: a.File, Line: a.Line, Kind: a.Kind, Text: a.Text, Focus: true})
	}
	for _, h := range m.step.Hotspots {
		if h.Line > 0 {
			out = append(out, Note{File: h.File, Line: h.Line, Kind: "hotspot", Text: h.Q, Focus: true})
		}
	}
	round := max(m.review.Round, 1)
	for _, c := range m.review.Comments {
		start, _, err := state.ParseLines(c.Lines)
		if err != nil || start == 0 || max(c.Round, 1) != round {
			continue
		}
		out = append(out, Note{File: c.File, Line: start, Kind: "comment", Label: string(c.Severity), Text: c.Body, Dim: c.Resolved})
	}
	out = append(out, m.pendingNotes()...)
	for _, d := range m.review.Discussions {
		if d.File == "" || d.OldLine {
			continue
		}
		body, _, _ := strings.Cut(d.Body, "\n")
		out = append(out, Note{File: d.File, Line: d.Line, Kind: "mr", Label: "@" + d.Author, Text: body, Dim: d.Resolved})
	}
	return out
}

func (m *model) pending(e inbox.Event) bool {
	if m.step == nil && e.Step != "" || m.step != nil && e.Step != m.step.ID {
		return false
	}
	switch e.Kind {
	case inbox.KindMessage, inbox.KindExplain, inbox.KindSkip:
		return e.Time.After(m.lastWait)
	}
	return false
}

func (m *model) pendingNotes() []Note {
	var out []Note
	for _, e := range m.events {
		if e.Kind != inbox.KindExplain || !m.pending(e) {
			continue
		}
		if start, _, err := state.ParseLines(e.Lines); err == nil && start > 0 {
			out = append(out, Note{File: e.File, Line: start, Kind: "pending", Text: "агент поясняет…", Focus: true})
		}
	}
	return out
}

func (m *model) useSplit() bool {
	return m.splitView && m.mainWidth() >= minSplitWidth
}

func (m *model) relist() {
	keep := m.current()
	m.disp = expandNotes(m.rows, m.mainWidth()-noteIndent)
	m.split = pairRows(m.disp)
	m.list = m.list[:0]
	if m.useSplit() {
		for _, r := range m.split {
			if r.Full != nil {
				m.list = append(m.list, item{File: r.Full.File, Line: r.Full.Line, HunkStart: r.Full.HunkStart, Note: r.Full.NoteHead, FileHead: r.Full.Kind == RowFile})
				continue
			}
			m.list = append(m.list, item{File: r.File, Line: r.Line, HunkStart: r.HunkStart})
		}
	} else {
		for _, r := range m.disp {
			m.list = append(m.list, item{File: r.File, Line: r.Line, HunkStart: r.HunkStart, Note: r.NoteHead, FileHead: r.Kind == RowFile})
		}
	}
	if keep.File != "" {
		m.focus(keep)
	}
	m.clamp()
}

func (m *model) current() item {
	if m.cursor < len(m.list) {
		return m.list[m.cursor]
	}
	return item{}
}

func (m *model) focus(it item) {
	for i, x := range m.list {
		if x.File == it.File && x.Line == it.Line && x.Note == it.Note {
			m.cursor = i
			return
		}
	}
}

func firstFocus(list []item) int {
	for i, it := range list {
		if it.HunkStart {
			return i
		}
	}
	for i, it := range list {
		if it.Line > 0 {
			return i
		}
	}
	return 0
}

func (m *model) Init() tea.Cmd { return tick() }

func (m *model) clock() time.Time {
	if m.now == nil {
		return time.Now()
	}
	return m.now()
}

func (m *model) refreshAgent() {
	if m.review == nil {
		return
	}
	if lw, ok := inbox.LastWait(m.store.ReviewDir(m.review.ID)); ok && !lw.Equal(m.lastWait) {
		m.lastWait = lw
		if m.step != nil && m.src != nil {
			m.rebuild(false)
		}
	}
	if since, ok := inbox.WaitingSince(m.store.ReviewDir(m.review.ID)); ok {
		m.agentWaiting, m.agentSince = true, since
		return
	}
	if m.agentWaiting || m.agentSince.IsZero() {
		m.agentSince = m.clock()
	}
	m.agentWaiting = false
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.relist()
	case reloadMsg:
		m.reload()
	case tickMsg:
		m.frame++
		m.refreshAgent()
		return m, tick()
	case editorDoneMsg:
		m.err = msg.err
	case tea.MouseMsg:
		return m, m.handleMouse(msg)
	case tea.KeyMsg:
		return m, m.handleKeys(msg)
	}
	return m, nil
}

func (m *model) handleKeys(msg tea.KeyMsg) tea.Cmd {
	if msg.Type != tea.KeyRunes || msg.Paste || len(msg.Runes) < 2 {
		if m.composing {
			m.handleCompose(msg)
			return nil
		}
		return m.handleKey(msg)
	}
	var cmds []tea.Cmd
	for i, r := range msg.Runes {
		if m.composing {
			m.input = append(m.input, msg.Runes[i:]...)
			break
		}
		cmds = append(cmds, m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}))
	}
	return tea.Batch(cmds...)
}

func (m *model) stepFiles() []string {
	var out []string
	for _, it := range m.list {
		if it.FileHead && (len(out) == 0 || out[len(out)-1] != it.File) {
			out = append(out, it.File)
		}
	}
	return out
}

func (m *model) jumpToFile(file string) {
	for i, it := range m.list {
		if it.FileHead && it.File == file {
			m.cursor, m.offset = i, i
			m.clamp()
			return
		}
	}
}

func (m *model) handleFilesKey(msg tea.KeyMsg) tea.Cmd {
	files := m.stepFiles()
	switch msg.String() {
	case "q", "ctrl+c":
		return tea.Quit
	case "j", "down":
		m.fileCursor = min(m.fileCursor+1, len(files)-1)
	case "k", "up":
		m.fileCursor = max(m.fileCursor-1, 0)
	case "enter":
		if m.fileCursor < len(files) {
			m.jumpToFile(files[m.fileCursor])
		}
		m.focusFiles = false
	case "esc", "f":
		m.focusFiles = false
	}
	return nil
}

func (m *model) handleKey(msg tea.KeyMsg) tea.Cmd {
	m.status = ""
	if m.focusFiles {
		return m.handleFilesKey(msg)
	}
	switch msg.String() {
	case "f":
		files := m.stepFiles()
		if len(files) == 0 {
			return nil
		}
		m.showPlan, m.focusFiles = true, true
		m.fileCursor = max(0, slices.Index(files, m.current().File))
		m.relist()
	case "}":
		m.jump(1, func(it item) bool { return it.FileHead })
	case "{":
		m.jump(-1, func(it item) bool { return it.FileHead })
	case "q", "ctrl+c":
		return tea.Quit
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
		m.cursor = len(m.list) - 1
		m.clamp()
	case "]":
		m.jump(1, func(it item) bool { return it.HunkStart })
	case "[":
		m.jump(-1, func(it item) bool { return it.HunkStart })
	case "n":
		m.jump(1, func(it item) bool { return it.Note })
	case "N":
		m.jump(-1, func(it item) bool { return it.Note })
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
	case "s":
		m.splitView = !m.splitView
		m.relist()
		if m.splitView && !m.useSplit() {
			m.status = "too narrow for split: widen the pane or hide the plan (p)"
		}
	case "p":
		m.showPlan = !m.showPlan
		m.relist()
	case "m":
		m.mouse = !m.mouse
		if m.mouse {
			return tea.EnableMouseCellMotion
		}
		return tea.DisableMouse
	case "v":
		m.visual = !m.visual
		m.anchor = m.cursor
	case "esc":
		m.visual = false
	case "c", "enter":
		m.startCompose(inbox.KindMessage)
	case "S":
		m.startCompose(inbox.KindSkip)
	case "?":
		m.explain()
	case ">", " ":
		m.emit(inbox.Event{Kind: inbox.KindNext})
	case "e":
		return m.openEditor()
	case "a":
		if os.Getenv("TMUX") != "" {
			if err := exec.Command("tmux", "last-window").Run(); err != nil {
				m.err = err
			}
		}
	}
	return nil
}

func (m *model) move(d int) {
	m.cursor += d
	m.clamp()
}

func (m *model) jump(dir int, match func(item) bool) {
	for i := m.cursor + dir; i >= 0 && i < len(m.list); i += dir {
		if match(m.list[i]) {
			m.cursor = i
			m.clamp()
			return
		}
	}
}

func (m *model) clamp() {
	m.cursor = max(0, min(m.cursor, len(m.list)-1))
	body := m.bodyHeight()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+body {
		m.offset = m.cursor - body + 1
	}
	m.offset = max(0, m.offset)
}

func (m *model) openEditor() tea.Cmd {
	it := m.current()
	if it.File == "" {
		return nil
	}
	dir := m.repo.Dir
	if m.review != nil && m.review.Worktree != "" {
		dir = m.review.Worktree
	}
	cmd := editorCmd(dir, it.File, max(it.Line, 1))
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
