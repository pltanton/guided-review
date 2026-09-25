package view

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/aplotnikov/guided-review/internal/config"
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

type rowsMsg struct {
	step string
	rows []Row
	err  error
}

const asyncFiles = 8

const tickEvery = 150 * time.Millisecond

func tick() tea.Cmd {
	return tea.Tick(tickEvery, func(time.Time) tea.Msg { return tickMsg{} })
}

type editorDoneMsg struct{ err error }

type model struct {
	ctx    context.Context
	store  state.Store
	repo   gitx.Repo
	src    *gitSource
	review *state.Review
	step   *state.Step
	rows   []Row
	lines  []line
	events []inbox.Event

	cursor, offset int
	width, height  int
	context        int

	splitView, showPlan, mouse bool
	visual, dragging           bool
	anchor                     int

	focusFiles bool
	fileCursor int
	viewStep   string

	showRemoved bool
	unfolded    map[string]bool
	algo        string
	loading     string
	returnPane  string
	closed      bool
	preview     string
	previewTop  int
	runGr       func(args ...string) (string, error)

	chatSize int
	chatTop  int
	chatG    bool

	col        int
	pendingKey string
	km         *keymap
	help       bool
	helpTop    int
	baseCtx    int
	popup      *popup
	popupStack []*popup
	lspDo      func(kind, file string, line, col int) tea.Cmd
	peekFile   func(path string) []string
	lspBusy    string
	lsp        *lspManager
	lspServers map[string][]string
	reveal     map[string][][2]int

	composing   bool
	composeKind string
	cmdMode     rune
	history     []string
	histIdx     int
	search      string
	input       []rune
	inputPos    int
	composeRef  int
	anchorFile  string
	anchorLines string

	send   func(inbox.Event) error
	status string
	err    error

	now          func() time.Time
	agentWaiting bool
	agentIdle    bool
	agentSince   time.Time
	lastWait     time.Time
	frame        int
}

type Options struct {
	Store      state.Store
	Repo       gitx.Repo
	Config     config.Config
	ReturnPane string
}

func newModel(ctx context.Context, o Options) *model {
	m := &model{
		ctx:        ctx,
		store:      o.Store,
		repo:       o.Repo,
		returnPane: o.ReturnPane,
		algo:       gitx.DefaultDiffAlgorithm,
	}
	m.applyConfig(o.Config)
	m.lspDo = m.defaultLSP
	m.runGr = func(args ...string) (string, error) {
		bin, err := os.Executable()
		if err != nil {
			return "", err
		}
		cmd := exec.CommandContext(ctx, bin, args...)
		cmd.Dir = o.Repo.Dir
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
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
		m.closed = m.review != nil && errors.Is(err, state.ErrNoReview)
		m.review, m.step, m.rows, m.lines, m.err = nil, nil, nil, nil, err
		return
	}
	if m.src == nil || m.src.base != r.DiffBase() || m.src.head != r.HeadSHA {
		m.src = newGitSource(m.ctx, m.repo, m.algo, r.DiffBase(), r.HeadSHA)
	}
	m.events, _ = inbox.All(m.store.ReviewDir(r.ID))
	m.lastWait, _ = inbox.LastWait(m.store.ReviewDir(r.ID))
	prev := ""
	if m.step != nil {
		prev = m.step.ID
	}
	m.review, m.err = r, nil
	target := r.Current
	if m.viewStep != "" && m.viewStep != r.Current && m.stepByID(m.viewStep) != nil {
		target = m.viewStep
	} else {
		m.viewStep = ""
	}
	m.step = m.stepByID(target)
	if m.step == nil {
		m.rows, m.lines = nil, nil
		return
	}
	changed := m.step.ID != prev
	if changed {
		m.context, m.cursor, m.offset, m.visual, m.reveal = m.baseCtx, 0, 0, false, nil
	}
	m.rebuild(changed)
}

func (m *model) rebuild(jumpToHunk bool) {
	rows, err := buildRows(m.src, *m.step, m.context, m.notes(), m.reveal)
	if err != nil {
		m.err = err
		return
	}
	keep := m.current()
	m.rows = rows
	m.relist()
	switch {
	case jumpToHunk:
		m.cursor = firstFocus(m.lines)
	case keep.File != "":
		m.focus(keep)
	}
	m.clamp()
}

func (m *model) notes() []Note {
	var out []Note
	for _, a := range m.step.Annotations {
		out = append(
			out,
			Note{File: a.File, Line: a.Line, Kind: a.Kind, Text: a.Text, Focus: true},
		)
	}
	for _, h := range m.step.Hotspots {
		if h.Line > 0 {
			out = append(
				out,
				Note{File: h.File, Line: h.Line, Kind: "hotspot", Text: h.Q, Focus: true},
			)
		}
	}
	round := max(m.review.Round, 1)
	for _, c := range m.review.Comments {
		start, _, err := state.ParseLines(c.Lines)
		if err != nil || start == 0 || max(c.Round, 1) != round {
			continue
		}
		out = append(
			out,
			Note{
				Ref:   c.ID,
				File:  c.File,
				Line:  start,
				Kind:  "comment",
				Label: fmt.Sprintf("#%d %s", c.ID, c.Severity),
				Text:  c.Body,
				Dim:   c.Resolved,
			},
		)
	}
	out = append(out, m.pendingNotes()...)
	for _, d := range m.review.Discussions {
		if d.File == "" || d.OldLine || d.Resolved {
			continue
		}
		body, _, _ := strings.Cut(d.Body, "\n")
		out = append(
			out,
			Note{File: d.File, Line: d.Line, Kind: "mr", Label: "@" + d.Author, Text: body},
		)
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
			out = append(
				out,
				Note{
					File:  e.File,
					Line:  start,
					Kind:  "pending",
					Text:  "agent is explaining…",
					Focus: true,
				},
			)
		}
	}
	return out
}

func (m *model) useSplit() bool {
	return m.splitView && m.mainWidth() >= minSplitWidth
}

func (m *model) relist() {
	keep := m.current()
	width := m.mainWidth() - noteIndent
	if m.useSplit() {
		m.lines = pairRows(expandNotes(m.rows, width))
	} else {
		rows := m.rows
		if !m.showRemoved {
			rows = foldRemoved(rows, m.unfolded)
		}
		m.lines = unifiedLines(expandNotes(rows, width))
	}
	if keep.File != "" {
		m.focus(keep)
	}
	m.clamp()
}

func (m *model) current() line {
	if m.cursor < len(m.lines) {
		return m.lines[m.cursor]
	}
	return line{}
}

func (m *model) focus(it line) {
	for i, x := range m.lines {
		if x.File == it.File && x.Line == it.Line && x.NoteHead == it.NoteHead {
			m.cursor = i
			return
		}
	}
}

func firstFocus(list []line) int {
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

func (m *model) applyConfig(c config.Config) {
	var err error
	m.km, err = newKeymap(c.Keys)
	m.err = err
	m.splitView, m.showPlan, m.mouse = c.View.Split, !c.View.HidePlan, !c.View.NoMouse
	m.baseCtx = cmp.Or(c.View.Context, defaultContext)
	m.context = m.baseCtx
	if c.View.Style != "" {
		styleName = c.View.Style
	}
	m.algo = cmp.Or(c.Diff, m.algo)
	m.lspServers = c.LSP
}

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
		m.agentWaiting, m.agentIdle, m.agentSince = true, false, since
		return
	}
	idle, ok := inbox.IdleSince(m.store.ReviewDir(m.review.ID))
	m.agentIdle = ok && idle.After(m.lastWait)
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
		if m.closed {
			return m, tea.Quit
		}
	case lspMsg:
		m.handleLSP(msg)
	case rowsMsg:
		if m.step == nil || msg.step != m.step.ID || msg.step != m.loading {
			return m, nil
		}
		m.loading, m.err = "", msg.err
		m.rows = msg.rows
		m.relist()
		m.cursor = firstFocus(m.lines)
		m.clamp()
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
			return m.handleCompose(msg)
		}
		return m.handleKey(msg)
	}
	var cmds []tea.Cmd
	for i, r := range msg.Runes {
		if m.composing {
			m.insert(msg.Runes[i:])
			break
		}
		cmds = append(cmds, m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}))
	}
	return tea.Batch(cmds...)
}

func (m *model) stepFiles() []string {
	var out []string
	for _, it := range m.lines {
		if it.Kind == RowFile && (len(out) == 0 || out[len(out)-1] != it.File) {
			out = append(out, it.File)
		}
	}
	return out
}

func (m *model) jumpToFile(file string) {
	for i, it := range m.lines {
		if it.Kind == RowFile && it.File == file {
			m.cursor, m.offset = i, i
			m.clamp()
			return
		}
	}
}

func (m *model) hasFolds() bool {
	for _, r := range m.lines {
		if r.Kind == RowFold {
			return true
		}
	}
	return false
}

func (m *model) toggleFold() {
	cur := m.current()
	if cur.GapTo > 0 {
		if m.reveal == nil {
			m.reveal = map[string][][2]int{}
		}
		m.reveal[cur.File] = append(m.reveal[cur.File], [2]int{cur.GapFrom, cur.GapTo})
		m.status = fmt.Sprintf("showing lines %d–%d", cur.GapFrom, cur.GapTo)
		if m.src != nil {
			m.rebuild(false)
		}
		for i, it := range m.lines {
			if it.File == cur.File && it.Line == cur.GapFrom {
				m.cursor = i
				break
			}
		}
		m.clamp()
		return
	}
	key := cur.FoldKey
	if key == "" {
		m.status = "nothing to open here: o opens ⋯ hidden lines and ▸ folded removed blocks"
		return
	}
	if m.unfolded == nil {
		m.unfolded = map[string]bool{}
	}
	m.unfolded[key] = !m.unfolded[key]
	m.relist()
	for i, it := range m.lines {
		if it.FoldKey == key {
			m.cursor = i
			break
		}
	}
	m.clamp()
}

const (
	extraBoilerplate = "~boilerplate"
	extraGenerated   = "~generated"
	extraAll         = "~all"
)

func isExtra(id string) bool {
	return strings.HasPrefix(id, "~")
}

func (m *model) extraSteps() []state.Step {
	if m.review == nil {
		return nil
	}
	var boilerplate, generated, all []state.StepHunk
	for _, f := range m.review.Files {
		h := state.StepHunk{File: f.Path}
		all = append(all, h)
		switch f.Tier {
		case state.TierBoilerplate:
			boilerplate = append(boilerplate, h)
		case state.TierGenerated:
			generated = append(generated, h)
		}
	}
	var out []state.Step
	for _, e := range []struct {
		id, title string
		hunks     []state.StepHunk
	}{
		{extraBoilerplate, "boilerplate", boilerplate},
		{extraGenerated, "generated", generated},
		{extraAll, "all changes", all},
	} {
		if len(e.hunks) > 0 {
			out = append(
				out,
				state.Step{
					ID:     e.id,
					Title:  e.title,
					Kind:   "extra",
					Hunks:  e.hunks,
					Status: state.StatusPending,
				},
			)
		}
	}
	return out
}

func (m *model) stepByID(id string) *state.Step {
	if m.review == nil {
		return nil
	}
	if st := m.review.Step(id); st != nil {
		return st
	}
	for _, st := range m.extraSteps() {
		if st.ID == id {
			return &st
		}
	}
	return nil
}

func (m *model) stepIDs() []string {
	var ids []string
	for _, st := range m.review.Steps {
		ids = append(ids, st.ID)
	}
	for _, st := range m.extraSteps() {
		ids = append(ids, st.ID)
	}
	return ids
}

func (m *model) showStep(id string) tea.Cmd {
	st := m.stepByID(id)
	if st == nil {
		return nil
	}
	m.viewStep = id
	if id == m.review.Current {
		m.viewStep = ""
	}
	m.step = st
	m.visual, m.cursor, m.offset, m.context, m.reveal = false, 0, 0, m.baseCtx, nil
	if m.src == nil {
		return nil
	}
	if len(st.Hunks) <= asyncFiles {
		m.loading = ""
		m.rebuild(true)
		return nil
	}
	m.loading, m.rows = st.ID, nil
	m.relist()
	src, step, context, notes := m.src, *st, m.context, m.notes()
	return func() tea.Msg {
		rows, err := buildRows(src, step, context, notes, nil)
		return rowsMsg{step: step.ID, rows: rows, err: err}
	}
}

func (m *model) shiftStep(d int) tea.Cmd {
	if m.step == nil {
		return nil
	}
	ids := m.stepIDs()
	if i := slices.Index(ids, m.step.ID) + d; i >= 0 && i < len(ids) {
		return m.showStep(ids[i])
	}
	return nil
}

func (m *model) next() {
	if m.viewStep != "" {
		m.status = "viewing an earlier step: esc to return, then >"
		return
	}
	m.emit(inbox.Event{Kind: inbox.KindNext})
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

func (m *model) publish() {
	if m.review == nil || m.runGr == nil {
		return
	}
	if m.preview == "" {
		if m.review.Publish == nil {
			m.status = "nothing prepared: the agent prepares the publication when the review is done"
			return
		}
		out, err := m.runGr("publish", "--dry-run")
		if err != nil {
			m.err = fmt.Errorf("%v: %s", err, strings.TrimSpace(out))
			return
		}
		m.preview, m.previewTop = out, 0
		return
	}
	out, err := m.runGr("publish")
	m.preview = ""
	if err != nil {
		m.err = fmt.Errorf("publish failed: %v: %s", err, strings.TrimSpace(out))
		return
	}
	first, _, _ := strings.Cut(strings.TrimSpace(out), "\n")
	m.emit(inbox.Event{Kind: inbox.KindPublished, Text: first})
	m.status = first
}

func (m *model) handlePreviewKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "P":
		m.publish()
	case "esc", "q":
		m.preview = ""
	case "j", "down":
		m.previewTop++
	case "k", "up":
		m.previewTop = max(m.previewTop-1, 0)
	}
	return nil
}

func (m *model) handleKey(msg tea.KeyMsg) tea.Cmd {
	m.status = ""
	switch {
	case m.help:
		return m.handleHelpKey(msg)
	case m.preview != "":
		return m.handlePreviewKey(msg)
	case m.popup != nil:
		return m.handlePopupKey(msg)
	case m.focusFiles:
		return m.handleFilesKey(msg)
	case m.chatSize == 2:
		if cmd, handled := m.handleChatKey(msg); handled {
			return cmd
		}
	}
	return m.dispatch(msg.String())
}

func (m *model) move(d int) {
	m.cursor += d
	m.clamp()
}

func (m *model) jump(dir int, match func(line) bool) {
	for i := m.cursor + dir; i >= 0 && i < len(m.lines); i += dir {
		if match(m.lines[i]) {
			m.cursor = i
			m.clamp()
			return
		}
	}
}

func (m *model) clamp() {
	m.cursor = max(0, min(m.cursor, len(m.lines)-1))
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
		cmd = exec.Command(
			"tmux",
			"display-popup",
			"-E",
			"-w",
			"90%",
			"-h",
			"90%",
			"-d",
			dir,
			script,
		)
	} else {
		cmd = exec.Command("sh", "-c", script)
	}
	cmd.Dir = dir
	return cmd
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func focusAgent(pane string) error {
	if os.Getenv("TMUX") == "" {
		return nil
	}
	if pane == "" {
		return exec.Command("tmux", "last-window").Run()
	}
	if err := exec.Command("tmux", "select-window", "-t", pane).Run(); err != nil {
		return err
	}
	return exec.Command("tmux", "select-pane", "-t", pane).Run()
}

func (m *model) handleChatKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	k := msg.String()
	if k != "g" {
		m.chatG = false
	}
	switch k {
	case "j", "down":
		m.chatTop = max(m.chatTop-1, 0)
	case "k", "up":
		m.chatTop++
	case "ctrl+d":
		m.chatTop = max(m.chatTop-10, 0)
	case "ctrl+u":
		m.chatTop += 10
	case "G", "end":
		m.chatTop = 0
	case "g":
		if m.chatG {
			m.chatTop, m.chatG = 1<<20, false
		} else {
			m.chatG = true
		}
	case "esc":
		m.chatSize, m.chatTop = 0, 0
	default:
		return nil, false
	}
	return nil, true
}
