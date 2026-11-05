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

	"github.com/pltanton/guided-review/internal/config"
	"github.com/pltanton/guided-review/internal/gitx"
	"github.com/pltanton/guided-review/internal/inbox"
	"github.com/pltanton/guided-review/internal/state"
)

const (
	DefaultContext = 3
	minSplitWidth  = 80
	minCodeWidth   = 70
	messageLines   = 5
)

type (
	reloadMsg     struct{}
	tickMsg       struct{}
	editorDoneMsg struct{ err error }
	rowsMsg       struct {
		gen  int
		rows []Row
		err  error
	}
)

const (
	asyncFiles = 8
	tickEvery  = 150 * time.Millisecond
)

func tick() tea.Cmd {
	return tea.Tick(tickEvery, func(time.Time) tea.Msg { return tickMsg{} })
}

type model struct {
	ctx    context.Context
	store  state.Store
	repo   gitx.Repo
	src    *gitSource
	review *state.Review
	rows   []Row
	lines  []line
	events []inbox.Event

	cursor, offset int
	width, height  int
	context        int

	splitView, mouse bool
	visual, dragging bool
	anchor           int

	focusFiles bool
	fileCursor int
	fileTop    int
	fileFollow string
	tierOpen   map[state.Tier]bool

	since      string
	sinceN     int
	scoped     map[string]bool
	commits    []gitx.LogEntry
	commitCard bool
	commitSel  int

	showRemoved   bool
	unfolded      map[string]bool
	folded        map[string]bool
	algo          string
	loading       bool
	loadGen       int
	loadCmd       tea.Cmd
	returnPane    string
	interrupted   bool
	tmux          func(args ...string) error
	clip          func(text string) error
	closed        bool
	preview       string
	previewTop    int
	previewSel    int
	previewFollow bool
	composeThread string
	runGr         func(args ...string) (string, error)

	sideW    int
	resizing bool
	chatTop  int

	chatFocus, chatVisual, chatDrag bool
	chatCursor, chatAnchor          int
	chatFrom                        int

	col          int
	hscroll      int
	nowrap       bool
	pendingKey   string
	count        string
	km           *keymap
	help         bool
	helpAll      bool
	modalY       int
	helpTop      int
	baseCtx      int
	popup        *popup
	popupStack   []*popup
	popupForward []*popup
	jumpFrom     *line
	lspTarget    *lspLoc
	lspDo        func(kind, file string, line, col int) tea.Cmd
	hereDo       func(hereReq) tea.Cmd
	here         hereState
	peekFile     func(path string) []string
	lspBusy      string
	lsp          *lspManager
	lspServers   map[string][]string
	reveal       map[string][][2]int

	composing     bool
	composeKind   string
	cmdMode       rune
	history       []string
	histIdx       int
	paletteSel    int
	search        string
	input         []rune
	inputPos      int
	composeRef    int
	raw           bool
	deleteArmed   int
	newsOpen      bool
	newsAll       bool
	newsTop       int
	newsFile      string
	newsChecked   bool
	themeSel      int
	themeWas      string
	threadCard    string
	threadCardSel int
	pub           *publishCard
	finishCard    bool
	finishSel     int
	approvePick   int
	autoVerdict   bool
	seen          map[seenKey]bool
	rawSeverity   state.Severity
	anchorFile    string
	anchorLines   string
	inlineAt      int
	sugOn         bool
	sugFocus      bool
	altInput      []rune
	altPos        int
	listW         int
	chatting      bool
	chatAbout     string
	chatTopic     string

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
	cfgErr := m.applyConfig(o.Config)
	if dir, err := os.UserCacheDir(); err == nil {
		m.newsFile = filepath.Join(dir, "guided-review", "seen-version")
	}
	m.lspDo = m.defaultLSP
	m.hereDo = m.defaultHere
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
	if cfgErr != nil {
		m.err = cfgErr
	}
	return m
}

func (m *model) reload() {
	r, err := m.store.LoadCurrent()
	if err != nil {
		m.closed = m.review != nil && errors.Is(err, state.ErrNoReview)
		m.review, m.rows, m.lines, m.err = nil, nil, nil, err
		m.closeCards()
		return
	}
	first := m.review == nil || m.review.ID != r.ID || m.review.HeadSHA != r.HeadSHA
	m.review = r
	if m.src == nil || m.src.base != m.scopeBase() || m.src.head != r.HeadSHA {
		m.src = newGitSource(m.ctx, m.repo, m.algo, m.scopeBase(), r.HeadSHA)
		m.rescope()
	}
	m.events, _ = inbox.All(m.store.ReviewDir(r.ID))
	m.lastWait, _ = inbox.LastWait(m.store.ReviewDir(r.ID))
	m.review, m.err = r, nil
	if first {
		m.context, m.cursor, m.offset, m.visual, m.reveal = m.baseCtx, 0, 0, false, nil
		m.hscroll = 0
	}
	// A reader still at the start follows the agent's order when it arrives instead of
	// staying on the file that was first only by path.
	atStart := !first && len(m.lines) > 0 && m.offset == 0 && m.cursor == firstFocus(m.lines)
	if first && len(r.Files) > asyncFiles {
		m.loadGen++
		m.loading, m.loadCmd = true, m.loadAsync(m.loadGen)
	} else {
		m.rebuild(first || atStart)
	}
	m.refreshDetail()
	m.refreshPreview()
}

func (m *model) loadAsync(gen int) tea.Cmd {
	src, files, hotspots, context, notes, open := m.src, m.files(), m.review.Hotspots,
		m.context, m.notes(), m.tierOpen
	return func() tea.Msg {
		rows, err := buildRows(src, files, hotspots, context, notes, nil, open)
		return rowsMsg{gen: gen, rows: rows, err: err}
	}
}

func (m *model) closeCards() {
	m.finishCard, m.pub, m.threadCard, m.focusFiles, m.commitCard = false, nil, "", false, false
}

func (m *model) rebuild(jumpToHunk bool) {
	rows, err := buildRows(m.src, m.files(), m.review.Hotspots, m.context, m.notes(),
		m.reveal, m.tierOpen)
	if err != nil {
		m.err = err
		return
	}
	keep := m.current()
	m.rows = rows
	m.relist()
	switch {
	case jumpToHunk:
		m.cursor, m.offset = firstFocus(m.lines), 0
	case keep.File != "":
		m.focus(keep)
	}
	m.clamp()
}

func (m *model) notes() []Note {
	r := m.review
	var out []Note
	if r.Briefing != "" {
		out = append(out, Note{Top: true, Kind: "note", Label: "BRIEFING", Text: r.Briefing})
	}
	if r.Round > 1 && len(m.myThreads()) > 0 {
		out = append(out, m.threadsIntro())
	}
	for _, a := range r.Annotations {
		out = append(out, Note{
			File: a.File, Line: a.Line, To: a.To, Kind: a.Kind, Text: a.Text, Focus: true,
		})
	}
	for i, h := range r.Hotspots {
		label := ""
		switch {
		case h.Checked && h.Comment > 0:
			label = fmt.Sprintf("✓ DONE #%d", h.Comment)
		case h.Checked:
			label = "✓ DONE"
		}
		n := Note{File: h.File, Line: h.Line, Kind: "hotspot", Text: h.Q, Focus: true,
			Label: label, Dim: h.Checked, Risk: i + 1}
		if h.Line == 0 {
			n.Top, n.Focus = true, false
		}
		out = append(out, n)
	}
	round := max(r.Round, 1)
	for _, c := range r.Comments {
		start, end, err := state.ParseLines(c.Lines)
		if err != nil || start == 0 || max(c.Round, 1) != round {
			continue
		}
		out = append(out, Note{
			Ref: c.ID, File: c.File, Line: start, To: end, Kind: "comment",
			Label: resolvedLabel(fmt.Sprintf("#%d %s", c.ID, c.Severity), c.Resolved),
			Text:  c.Body, Dim: c.Resolved,
		})
	}
	for _, e := range m.events {
		if e.Kind != inbox.KindExplain || !m.pending(e) {
			continue
		}
		if start, _, err := state.ParseLines(e.Lines); err == nil && start > 0 {
			out = append(out, Note{
				File: e.File, Line: start, Kind: "pending", Focus: true,
				Text: "agent is explaining…",
			})
		}
	}
	for _, d := range r.Discussions {
		if d.File == "" || d.OldLine || d.Resolved || m.mine(d) {
			continue
		}
		body, _, _ := strings.Cut(d.Body, "\n")
		label, dim := m.threadBadge(d)
		out = append(out, Note{
			File: d.File, Line: d.Line, Kind: "mr", Label: label, Text: body, Dim: dim,
			Thread: d.ID,
		})
	}
	return append(out, m.threadNotes()...)
}

func (m *model) pending(e inbox.Event) bool {
	switch e.Kind {
	case inbox.KindMessage, inbox.KindExplain, inbox.KindAsk:
		return e.Time.After(m.lastWait)
	}
	return false
}

func (m *model) useSplit() bool {
	return m.splitView && m.mainWidth() >= minSplitWidth
}

func (m *model) relist() {
	keep := m.current()
	m.listW = m.mainWidth()
	width := m.listW - noteIndent
	if m.useSplit() {
		m.lines = pairRows(expandNotes(m.rows, width, m.folded))
	} else {
		rows := m.rows
		if !m.showRemoved {
			rows = foldRemoved(slices.DeleteFunc(slices.Clone(rows), func(r Row) bool {
				return r.RenameHide
			}), m.unfolded)
		}
		m.lines = unifiedLines(expandNotes(rows, width, m.folded))
	}
	if keep.File != "" {
		m.focus(keep)
	}
	m.clamp()
}

func (m *model) act() tea.Cmd {
	cur := m.current()
	switch {
	case m.review == nil || len(m.lines) == 0 || m.visual:
		m.startComment()
	case cur.Kind == RowFile && cur.FoldKey != "":
		m.toggleTier(state.Tier(cur.FoldKey))
	case cur.Kind == RowGap || cur.Kind == RowFold:
		m.toggleFold()
	case cur.Kind == RowNote && cur.Thread != "":
		m.threadCard, m.threadCardSel = cur.Thread, 0
	case cur.Kind == RowNote && cur.Ref > 0:
		m.startEdit()
	case cur.Kind == RowNote && agentKinds[cur.NoteKind]:
		m.noteDetails()
	default:
		m.startComment()
	}
	return nil
}

func (m *model) current() line {
	if m.cursor < len(m.lines) {
		return m.lines[m.cursor]
	}
	return line{}
}

// focus puts the cursor back on the same row after the rows were rebuilt.
func (m *model) focus(it line) {
	m.seek(func(x line) bool {
		if x.File != it.File || x.Kind != it.Kind || x.Pair != it.Pair {
			return false
		}
		switch x.Kind {
		case RowNote:
			return noteKey(x.Row) == noteKey(it.Row) && x.NoteHead == it.NoteHead
		case RowRemoved:
			return x.OldLine == it.OldLine
		}
		return x.Line == it.Line
	})
}

func (m *model) seek(match func(line) bool) bool {
	i := slices.IndexFunc(m.lines, match)
	if i >= 0 {
		m.cursor = i
	}
	return i >= 0
}

func firstFocus(list []line) int {
	if i := slices.IndexFunc(list, func(l line) bool { return l.HunkStart }); i >= 0 {
		return i
	}
	return max(slices.IndexFunc(list, func(l line) bool { return l.Line > 0 }), 0)
}

func (m *model) Init() tea.Cmd {
	cmd := m.loadCmd
	m.loadCmd = nil
	return tea.Batch(tick(), cmd)
}

func (m *model) applyConfig(c config.Config) error {
	var err error
	m.km, err = newKeymap(c.Keys)
	m.splitView, m.mouse = c.View.Split, !c.View.NoMouse
	m.nowrap = c.View.NoWrap
	m.baseCtx = cmp.Or(c.View.Context, DefaultContext)
	m.context = m.baseCtx
	if c.View.Theme != "" {
		if terr := applyTheme(c.View.Theme); terr != nil {
			err = errors.Join(err, terr)
		}
	}
	if c.View.Style != "" {
		styleName = c.View.Style
	}
	m.algo = cmp.Or(c.Diff, m.algo)
	m.lspServers = c.LSP
	return err
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
		if m.src != nil {
			m.rebuild(false)
		}
	}
	stopped := m.agentWaiting || m.agentIdle
	if inbox.Waiting(m.store.ReviewDir(m.review.ID)) {
		m.agentWaiting, m.agentIdle = true, false
	} else {
		idle, ok := inbox.IdleSince(m.store.ReviewDir(m.review.ID))
		m.agentIdle = ok && idle.After(m.lastWait)
		if m.agentWaiting || m.agentSince.IsZero() {
			m.agentSince = m.clock()
		}
		m.agentWaiting = false
	}
	if stopped != (m.agentWaiting || m.agentIdle) && m.src != nil && len(m.myThreads()) > 0 {
		m.rebuild(false)
	}
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	_, cmd := m.update(msg)
	return m, tea.Batch(cmd, m.followHere())
}

func (m *model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case hereMsg:
		return m, m.handleHere(msg)
	case hereTickMsg:
		return m, m.hereTick(msg.seq)
	case hereRetryMsg:
		if msg.req.file == m.here.file && m.hereDo != nil {
			return m, m.hereDo(msg.req)
		}
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.relist()
	case reloadMsg:
		m.reload()
		if m.closed {
			return m, tea.Quit
		}
		if cmd := m.loadCmd; cmd != nil {
			m.loadCmd = nil
			return m, cmd
		}
	case lspMsg:
		m.handleLSP(msg)
	case rowsMsg:
		if m.review == nil || msg.gen != m.loadGen {
			return m, nil
		}
		m.loading, m.err = false, msg.err
		m.rows = msg.rows
		m.relist()
		m.cursor = firstFocus(m.lines)
		m.clamp()
	case publishedMsg:
		if m.pub != nil {
			m.pub.running, m.pub.out, m.pub.err, m.pub.done = false, msg.out, msg.err, msg.err == nil
			m.pub.sel = 0
		}
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
	case fmt.Stringer:
		// bubbletea v1 hands modified keys over as its unexported unknownCSISequenceMsg.
		if mod, ok := modifiedEnter(msg.String()); ok {
			if mod == 1 && !m.composing {
				return m, m.shiftEnter()
			}
			alt := mod&2 != 0 || m.composing && mod != 0
			return m, m.handleKeys(tea.KeyMsg{Type: tea.KeyEnter, Alt: alt})
		}
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

func (m *model) shownFiles() []string {
	var out []string
	for i := range m.lines {
		if it := &m.lines[i]; it.Kind == RowFile && (len(out) == 0 || out[len(out)-1] != it.File) {
			out = append(out, it.File)
		}
	}
	return out
}

func (m *model) jumpToFile(file string) {
	if m.seek(func(l line) bool { return l.Kind == RowFile && l.File == file }) {
		m.offset = m.cursor
		m.clamp()
	}
}

func (m *model) hasFolds() bool {
	return slices.ContainsFunc(m.lines, func(l line) bool { return l.Kind == RowFold })
}

func (m *model) toggleFold() {
	cur := m.current()
	if cur.Kind == RowGap {
		if m.reveal == nil {
			m.reveal = map[string][][2]int{}
		}
		m.reveal[cur.File] = append(m.reveal[cur.File], [2]int{cur.GapFrom, cur.GapTo})
		m.status = fmt.Sprintf("showing lines %d–%d", cur.GapFrom, cur.GapTo)
		if m.src != nil {
			m.rebuild(false)
		}
		m.seek(func(l line) bool { return l.File == cur.File && l.Line == cur.GapFrom })
		m.clamp()
		return
	}
	if cur.Kind == RowNote {
		key := noteKey(cur.Row)
		if m.folded == nil {
			m.folded = map[string]bool{}
		}
		m.folded[key] = !m.folded[key]
		m.relist()
		m.seek(func(l line) bool { return l.NoteHead && noteKey(l.Row) == key })
		m.clamp()
		return
	}
	key := cur.FoldKey
	if key == "" {
		m.status = "nothing to open here: o opens ⋯ hidden lines, ▸ folded blocks and notes"
		return
	}
	if m.unfolded == nil {
		m.unfolded = map[string]bool{}
	}
	m.unfolded[key] = !m.unfolded[key]
	m.relist()
	m.seek(func(l line) bool { return l.FoldKey == key })
	m.clamp()
}

func (m *model) toggleRisk(n int) {
	if m.review == nil || n < 1 || n > len(m.review.Hotspots) {
		m.status = "put the cursor on a risk (RISK) to check it off"
		return
	}
	h := &m.review.Hotspots[n-1]
	args := []string{"check"}
	if h.Checked {
		args = append(args, "--undo")
	}
	if m.runGr != nil {
		out, err := m.runGr(append(args, fmt.Sprint(n))...)
		if err != nil {
			m.err = fmt.Errorf("%v: %s", err, strings.TrimSpace(out))
			return
		}
	}
	h.Checked = !h.Checked
	m.status = "risk checked off"
	if !h.Checked {
		m.status = "risk open again"
	}
	if m.src != nil {
		m.rebuild(false)
	}
}

func (m *model) handleFilesKey(msg tea.KeyMsg) tea.Cmd {
	items := m.treeItems()
	switch m.keys().name(msg.String()) {
	case "quit":
		return tea.Quit
	case "down":
		m.fileCursor = max(0, min(m.fileCursor+1, len(items)-1))
	case "up":
		m.fileCursor = max(m.fileCursor-1, 0)
	case "top":
		m.fileCursor = 0
	case "bottom":
		m.fileCursor = max(len(items)-1, 0)
	case "act", "message", "open":
		if m.fileCursor >= 0 && m.fileCursor < len(items) {
			m.openTreeItem(items[m.fileCursor])
		}
	case "back", "files":
		m.focusFiles = false
	}
	return nil
}

func (m *model) finish() tea.Cmd {
	if m.review == nil || m.runGr == nil {
		return nil
	}
	if m.review.Publish == nil {
		m.openFinish()
		return nil
	}
	if m.preview == "" {
		out, err := m.runGr("export", "--dry-run")
		switch {
		case err != nil:
			m.err = fmt.Errorf("%v: %s", err, strings.TrimSpace(out))
		case strings.TrimSpace(out) == "":
			m.status = fmt.Sprintf("round %d is published already: a new comment or verdict goes out with %s",
				max(m.review.Round, 1), m.keys().key("finish"))
		default:
			m.preview, m.previewTop = out, 0
		}
		return nil
	}
	out, err := m.runGr("export")
	out = strings.TrimSpace(out)
	if err != nil {
		m.err = fmt.Errorf("export failed: %v: %s", err, out)
		return nil
	}
	m.preview = ""
	if m.review.MR != nil {
		m.pub = &publishCard{dir: out}
		return nil
	}
	return m.handOver(out)
}

func (m *model) handOver(text string) tea.Cmd {
	if m.emit(inbox.Event{Kind: inbox.KindFinished, Text: text}); m.err != nil {
		return nil
	}
	return tea.Quit
}

func (m *model) handleKey(msg tea.KeyMsg) tea.Cmd {
	m.status = ""
	if m.keys().name(msg.String()) != "delete-comment" {
		m.deleteArmed = 0
	}
	filtering := m.popup != nil && m.popup.filtering
	overlay := m.review != nil && m.preview == ""
	if k := msg.String(); overlay && !m.help && !filtering && (k == "?" || k == "f1") {
		m.help, m.helpAll, m.helpTop = true, false, 0
		return nil
	}
	switch {
	case m.help:
		return m.handleHelpKey(msg)
	case m.finishCard:
		return m.handleFinishCardKey(msg)
	case m.commitCard:
		return m.handleCommitsKey(msg)
	case m.pub != nil:
		return m.handlePublishKey(msg)
	case m.threadCard != "":
		return m.handleThreadCardKey(msg)
	case m.themeWas != "":
		return m.handleThemeKey(msg)
	case m.newsOpen:
		return m.handleNewsKey(msg)
	case m.preview != "":
		return m.handlePreviewKey(msg)
	case m.popup != nil:
		return m.handlePopupKey(msg)
	case m.focusFiles:
		return m.handleFilesKey(msg)
	case m.chatFocus:
		return m.handleChatKey(msg)
	}
	if i, ok := m.optionKey(msg.String()); ok {
		m.answer(i)
		return nil
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
	m.anchor = max(0, min(m.anchor, len(m.lines)-1))
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.height == 0 {
		m.offset = 0
		return
	}
	m.offset = max(0, m.offset, m.topFor(m.cursor, m.bodyHeight()))
}

func (m *model) openEditor() tea.Cmd {
	it := m.current()
	if it.File == "" {
		return nil
	}
	dir := m.repo.Dir
	if m.review != nil {
		dir = m.review.CodeDir(dir)
	}
	cmd := editorCmd(dir, it.File, max(it.Line, 1))
	return tea.ExecProcess(cmd, func(err error) tea.Msg { return editorDoneMsg{err} })
}

func editorCmd(dir, file string, line int) *exec.Cmd {
	editor := strings.Fields(cmp.Or(os.Getenv("VISUAL"), os.Getenv("EDITOR")))
	if len(editor) == 0 {
		editor = []string{"vi"}
	}
	target := fmt.Sprintf("+%d %s", line, shellQuote(file))
	if name := filepath.Base(editor[0]); name == "hx" || name == "helix" {
		target = shellQuote(fmt.Sprintf("%s:%d", file, line))
	}
	script := strings.Join(editor, " ") + " " + target
	var cmd *exec.Cmd
	if os.Getenv("TMUX") != "" {
		popup := []string{"display-popup", "-E", "-w", "90%", "-h", "90%", "-d", dir, script}
		cmd = exec.Command("tmux", popup...)
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

type seenKey struct {
	file    string
	line    int
	removed bool
}

func seenKeys(l *line) []seenKey {
	var keys []seenKey
	if l.Pair {
		if l.Left.Kind == RowRemoved {
			keys = append(keys, seenKey{l.File, l.Left.Line, true})
		}
		if l.Right.Kind == RowAdded {
			keys = append(keys, seenKey{l.File, l.Right.Line, false})
		}
		return keys
	}
	switch l.Kind {
	case RowAdded:
		keys = append(keys, seenKey{l.File, l.Line, false})
	case RowRemoved:
		keys = append(keys, seenKey{l.File, l.OldLine, true})
	case RowFold:
		for k := range l.FoldCount {
			keys = append(keys, seenKey{l.File, l.OldLine + k, true})
		}
	}
	return keys
}

func (m *model) markSeen(l *line) {
	if m.seen == nil {
		m.seen = map[seenKey]bool{}
	}
	for _, k := range seenKeys(l) {
		m.seen[k] = true
	}
}

// changeCounts counts the changed lines of each file and how many were not on screen yet.
func (m *model) changeCounts() (total, unseen map[string]int) {
	total, unseen = map[string]int{}, map[string]int{}
	for i := range m.rows {
		r := &m.rows[i]
		var k seenKey
		switch {
		case r.Kind == RowAdded:
			k = seenKey{r.File, r.Line, false}
		case r.Kind == RowRemoved && !r.Reformat && !r.RenameHide:
			k = seenKey{r.File, r.OldLine, true}
		default:
			continue
		}
		total[r.File]++
		if !m.seen[k] {
			unseen[r.File]++
		}
	}
	return total, unseen
}

// persistSeen records in the review the files whose changed lines were all on screen.
func (m *model) persistSeen() {
	if m.review == nil || m.loading || len(m.rows) == 0 || m.since != "" {
		return
	}
	_, unseen := m.changeCounts()
	shown := map[string]bool{}
	for _, r := range m.rows {
		if r.Kind == RowFile {
			shown[r.File] = true
		}
	}
	var done []string
	for i := range m.review.Files {
		f := &m.review.Files[i]
		if f.Tier == state.TierCore && !f.Seen && shown[f.Path] && unseen[f.Path] == 0 {
			f.Seen = true
			done = append(done, f.Path)
		}
	}
	if len(done) == 0 || m.store.Dir == "" {
		return
	}
	err := m.store.UpdateCurrent(func(r *state.Review) error {
		for _, path := range done {
			if f := r.File(path); f != nil {
				f.Seen = true
			}
		}
		return nil
	})
	if err != nil {
		m.err = err
	}
}

func resolvedLabel(label string, resolved bool) string {
	if resolved {
		return "✓ " + label
	}
	return label
}

func (m *model) unseen() int {
	_, unseen := m.changeCounts()
	n := 0
	for _, v := range unseen {
		n += v
	}
	return n
}
