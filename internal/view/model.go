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
	defaultContext = 3
	minSplitWidth  = 80
	minCodeWidth   = 70
	messageLines   = 5
)

type (
	reloadMsg     struct{}
	tickMsg       struct{}
	editorDoneMsg struct{ err error }
	rowsMsg       struct {
		step string
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
	step   *state.Step
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
	viewStep   string

	focusPlan  bool
	planCursor int
	planTop    int
	planFollow string
	planOpen   map[string]bool

	showRemoved   bool
	unfolded      map[string]bool
	folded        map[string]bool
	algo          string
	loading       string
	returnPane    string
	interrupted   bool
	tmux          func(args ...string) error
	clip          func(text string) error
	closed        bool
	preview       string
	previewTop    int
	previewSel    int
	previewFollow bool
	threads       bool
	threadSel     int
	threadTop     int
	threadFollow  bool
	composeThread string
	runGr         func(args ...string) (string, error)

	sideW    int
	resizing string
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
	chapterOpen  string
	modalY       int
	introShown   map[string]bool
	helpTop      int
	baseCtx      int
	popup        *popup
	popupStack   []*popup
	popupForward []*popup
	lspDo        func(kind, file string, line, col int) tea.Cmd
	peekFile     func(path string) []string
	lspBusy      string
	lsp          *lspManager
	lspServers   map[string][]string
	reveal       map[string][][2]int

	composing   bool
	composeKind string
	cmdMode     rune
	history     []string
	histIdx     int
	paletteSel  int
	search      string
	input       []rune
	inputPos    int
	composeRef  int
	raw         bool
	deleteArmed int
	gateOpen    bool
	staleSteps  []string
	staleSel    int
	finishCard  bool
	finishSel   int
	approvePick int
	autoVerdict bool
	gateSel     int
	notice      string
	lspStep     string
	seen        map[string]bool
	flow        []flowEntry
	rawSeverity state.Severity
	anchorFile  string
	anchorLines string
	inlineAt    int
	listW       int
	chatting    bool
	chatAbout   string
	chatTopic   string

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
	m.introOnce()
	m.rebuild(changed)
	m.refreshDetail()
	m.refreshPreview()
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
		out = append(out, Note{
			File: a.File, Line: a.Line, To: a.To, Kind: a.Kind, Text: a.Text, Focus: true,
		})
	}
	if m.step.Note != "" {
		out = append(out, Note{Top: true, Kind: "note", Text: m.step.Note})
	}
	for i, h := range m.step.Hotspots {
		label := ""
		switch {
		case h.Checked && h.Comment > 0:
			label = fmt.Sprintf("RISK ✓ #%d", h.Comment)
		case h.Checked:
			label = "RISK ✓"
		}
		if h.Line == 0 {
			out = append(out, Note{
				Top: true, Kind: "hotspot", Text: h.Q, Label: label, Dim: h.Checked, Risk: i + 1,
			})
		}
		if h.Line > 0 {
			out = append(out, Note{
				File: hotspotFile(m.step, h), Line: h.Line, Kind: "hotspot", Text: h.Q,
				Focus: true, Label: label, Dim: h.Checked, Risk: i + 1,
			})
		}
	}
	round := max(m.review.Round, 1)
	for _, c := range m.review.Comments {
		start, end, err := state.ParseLines(c.Lines)
		if err != nil || start == 0 || max(c.Round, 1) != round {
			continue
		}
		out = append(out, Note{
			Ref: c.ID, File: c.File, Line: start, To: end, Kind: "comment",
			Label: fmt.Sprintf("#%d %s", c.ID, c.Severity), Text: c.Body, Dim: c.Resolved,
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
	for _, d := range m.review.Discussions {
		if d.File == "" || d.OldLine || d.Resolved {
			continue
		}
		body, _, _ := strings.Cut(d.Body, "\n")
		out = append(out, Note{
			File: d.File, Line: d.Line, Kind: "mr", Label: "@" + d.Author, Text: body,
		})
	}
	return out
}

func (m *model) pending(e inbox.Event) bool {
	if m.step == nil && e.Step != "" || m.step != nil && e.Step != m.step.ID {
		return false
	}
	switch e.Kind {
	case inbox.KindMessage, inbox.KindExplain, inbox.KindSkip, inbox.KindAsk:
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
	if m.step != nil && len(m.rows) > 0 {
		m.lines = append(m.lines, line{Row: Row{Kind: RowEnd, Text: m.endText()}})
	}
	if keep.File != "" {
		m.focus(keep)
	}
	m.clamp()
}

func (m *model) endText() string {
	if m.viewStep != "" {
		return fmt.Sprintf("end of %s · enter → back to %s", m.step.ID, m.review.Current)
	}
	return fmt.Sprintf("end of %s · enter → next step", m.step.ID)
}

func (m *model) act() tea.Cmd {
	cur := m.current()
	switch {
	case m.step == nil || len(m.lines) == 0 || m.visual:
		m.startComment()
	case cur.Kind == RowEnd && m.viewStep != "":
		return m.back()
	case cur.Kind == RowEnd:
		m.next()
	case cur.GapTo > 0 || cur.Kind == RowFold:
		m.toggleFold()
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

func (m *model) focus(it line) {
	m.seek(func(x line) bool {
		return x.File == it.File && x.Line == it.Line && x.NoteHead == it.NoteHead
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

func (m *model) Init() tea.Cmd { return tick() }

func (m *model) applyConfig(c config.Config) {
	var err error
	m.km, err = newKeymap(c.Keys)
	m.err = err
	m.splitView, m.mouse = c.View.Split, !c.View.NoMouse
	m.nowrap = c.View.NoWrap
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
	_, cmd := m.update(msg)
	if m.step != nil && m.lspDo != nil && m.lspStep != m.step.ID && !isExtra(m.step.ID) {
		m.lspStep, m.flow = m.step.ID, nil
		cmd = tea.Batch(cmd, m.fetchFlow(), m.refreshLSPLater(1))
	}
	return m, cmd
}

func (m *model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case lspRefreshMsg:
		if m.step == nil || msg.step != m.step.ID || m.lspDo == nil {
			return m, nil
		}
		return m, tea.Batch(m.fetchFlow(), m.refreshLSPLater(msg.attempt+1))
	case flowMsg:
		if m.step != nil && msg.step == m.step.ID {
			m.flow = msg.entries
		}
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
	case fmt.Stringer:
		// bubbletea v1 hands modified keys over as its unexported unknownCSISequenceMsg.
		if mod, ok := modifiedEnter(msg.String()); ok {
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
	if cur.GapTo > 0 {
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
	add := func(id, title string, hunks []state.StepHunk) {
		if len(hunks) > 0 {
			out = append(out, state.Step{
				ID: id, Title: title, Kind: "extra", Hunks: hunks, Status: state.StatusPending,
			})
		}
	}
	add(extraBoilerplate, "boilerplate", boilerplate)
	add(extraGenerated, "generated", generated)
	add(extraAll, "all changes", all)
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
	m.hscroll = 0
	m.introOnce()
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
	if !m.gateOpen && (len(m.openRisks()) > 0 || m.unseen() > 0) {
		m.gateOpen, m.gateSel = true, len(m.step.Hotspots)
		if open := m.openRisks(); len(open) > 0 {
			m.gateSel = open[0]
		}
		return
	}
	m.gateOpen = false
	m.moveStep("next")
}

func (m *model) openRisks() []int {
	var out []int
	if m.step == nil {
		return nil
	}
	for i, h := range m.step.Hotspots {
		if !h.Checked {
			out = append(out, i)
		}
	}
	return out
}

func (m *model) toggleRisk(n int) {
	if m.step == nil || n < 1 || n > len(m.step.Hotspots) {
		m.status = "put the cursor on a risk (RISK) to check it off"
		return
	}
	h := &m.step.Hotspots[n-1]
	args := []string{"step", "check"}
	if h.Checked {
		args = append(args, "--undo")
	}
	if m.runGr != nil {
		out, err := m.runGr(append(args, m.step.ID, fmt.Sprint(n))...)
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

func (m *model) handleGateKey(msg tea.KeyMsg) tea.Cmd {
	risks := len(m.step.Hotspots)
	switch msg.String() {
	case "j", "down":
		m.gateSel = min(m.gateSel+1, risks)
	case "k", "up":
		m.gateSel = max(m.gateSel-1, 0)
	case "x":
		m.toggleRisk(m.gateSel + 1)
	case "enter":
		if m.gateSel >= risks {
			m.next()
			return nil
		}
		m.toggleRisk(m.gateSel + 1)
		if open := m.openRisks(); len(open) > 0 {
			m.gateSel = open[0]
		} else {
			m.gateSel = risks
		}
	case "esc", "q":
		m.gateOpen = false
	}
	return nil
}

func (m *model) moveStep(args ...string) {
	if m.runGr == nil {
		m.err = errors.New("no gr to move the step with")
		return
	}
	out, err := m.runGr(append([]string{"step"}, args...)...)
	out = strings.TrimSpace(out)
	switch {
	case err != nil:
		m.err = fmt.Errorf("%v: %s", err, out)
	case strings.HasPrefix(out, "all steps reviewed"):
		m.openFinish()
	}
}

func (m *model) handleFilesKey(msg tea.KeyMsg) tea.Cmd {
	files := m.stepFiles()
	switch m.keys().name(msg.String()) {
	case "quit":
		return tea.Quit
	case "down":
		m.fileCursor = min(m.fileCursor+1, len(files)-1)
	case "up":
		m.fileCursor = max(m.fileCursor-1, 0)
	case "top":
		m.fileCursor = 0
	case "bottom":
		m.fileCursor = max(len(files)-1, 0)
	case "act", "message", "open":
		if m.fileCursor < len(files) {
			m.jumpToFile(files[m.fileCursor])
		}
		m.focusFiles = false
	case "back", "files":
		m.focusFiles = false
	case "steps":
		m.focusFiles = false
		m.focusPlanPanel()
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
		if err != nil {
			m.err = fmt.Errorf("%v: %s", err, strings.TrimSpace(out))
			return nil
		}
		m.preview, m.previewTop = out, 0
		return nil
	}
	out, err := m.runGr("export")
	out = strings.TrimSpace(out)
	if err != nil {
		m.err = fmt.Errorf("export failed: %v: %s", err, out)
		return nil
	}
	m.preview = ""
	if m.emit(inbox.Event{Kind: inbox.KindFinished, Text: out}); m.err != nil {
		return nil
	}
	return tea.Quit
}

func (m *model) handleKey(msg tea.KeyMsg) tea.Cmd {
	m.status, m.notice = "", ""
	if m.keys().name(msg.String()) != "delete-comment" {
		m.deleteArmed = 0
	}
	filtering := m.popup != nil && m.popup.filtering
	if k := msg.String(); !m.help && !filtering && (k == "?" || k == "f1") {
		m.help, m.helpAll, m.helpTop = true, false, 0
		return nil
	}
	switch {
	case m.help:
		return m.handleHelpKey(msg)
	case m.chapterOpen != "":
		m.chapterOpen = ""
		return nil
	case m.gateOpen:
		return m.handleGateKey(msg)
	case m.finishCard:
		return m.handleFinishCardKey(msg)
	case len(m.staleSteps) > 0:
		return m.handleStaleKey(msg)
	case m.preview != "":
		return m.handlePreviewKey(msg)
	case m.threads && !m.composing:
		return m.handleThreadsKey(msg)
	case m.popup != nil:
		return m.handlePopupKey(msg)
	case m.focusFiles:
		return m.handleFilesKey(msg)
	case m.focusPlan:
		return m.handlePlanKey(msg)
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
	if m.cursor < m.offset {
		m.offset = m.cursor
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
	editor := cmp.Or(os.Getenv("VISUAL"), os.Getenv("EDITOR"), "vi")
	target := fmt.Sprintf("+%d %s", line, shellQuote(file))
	if name := filepath.Base(strings.Fields(editor)[0]); name == "hx" || name == "helix" {
		target = shellQuote(fmt.Sprintf("%s:%d", file, line))
	}
	script := editor + " " + target
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

func seenKeys(l line) []string {
	var keys []string
	if l.Pair {
		if l.Left.Kind == RowRemoved {
			keys = append(keys, fmt.Sprintf("-%s:%d", l.File, l.Left.Line))
		}
		if l.Right.Kind == RowAdded {
			keys = append(keys, fmt.Sprintf("+%s:%d", l.File, l.Right.Line))
		}
		return keys
	}
	switch l.Kind {
	case RowAdded:
		keys = append(keys, fmt.Sprintf("+%s:%d", l.File, l.Line))
	case RowRemoved:
		keys = append(keys, fmt.Sprintf("-%s:%d", l.File, l.OldLine))
	case RowFold:
		for k := range l.FoldCount {
			keys = append(keys, fmt.Sprintf("-%s:%d", l.File, l.OldLine+k))
		}
	}
	return keys
}

func (m *model) markSeen(l line) {
	if m.seen == nil {
		m.seen = map[string]bool{}
	}
	for _, k := range seenKeys(l) {
		m.seen[k] = true
	}
}

func (m *model) changedKeys() []string {
	var keys []string
	for _, r := range m.rows {
		switch {
		case r.Kind == RowAdded:
			keys = append(keys, fmt.Sprintf("+%s:%d", r.File, r.Line))
		case r.Kind == RowRemoved && !r.Reformat && !r.RenameHide:
			keys = append(keys, fmt.Sprintf("-%s:%d", r.File, r.OldLine))
		}
	}
	return keys
}

func (m *model) changedRows() int { return len(m.changedKeys()) }

func (m *model) unseen() int {
	n := 0
	for _, k := range m.changedKeys() {
		if !m.seen[k] {
			n++
		}
	}
	return n
}
