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
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/pltanton/guided-review/internal/diff"
	"github.com/pltanton/guided-review/internal/lsp"
)

var languages = map[string]string{
	".go": "go", ".kt": "kotlin", ".kts": "kotlin", ".py": "python",
	".ts": "typescript", ".tsx": "typescript", ".java": "java", ".rs": "rust",
}

var DefaultServers = map[string][]string{
	"go":         {"gopls"},
	"kotlin":     {"kotlin-lsp", "--stdio"},
	"python":     {"basedpyright-langserver", "--stdio"},
	"typescript": {"typescript-language-server", "--stdio"},
	"java":       {"jdtls"},
	"rust":       {"rust-analyzer"},
}

const lspTimeout = 90 * time.Second

const minPreviewWidth = 80

type lspLoc struct {
	Path string
	Line int
	End  int
	Text string
}

type lspMsg struct {
	kind  string
	locs  []lspLoc
	hover string
	err   error
}

type popup struct {
	kind   string
	title  string
	items  []lspLoc
	sel    int
	top    int
	lines  []string
	target int
	loc    lspLoc
	files  map[string][]string
	cursor int
	col    int
	gKey   bool
	refs   []lspLoc
}

type lspManager struct {
	mu      sync.Mutex
	root    string
	servers map[string][]string
	clients map[string]*lsp.Client
	opened  map[string]bool
}

func newLSPManager(root string, overrides map[string][]string) *lspManager {
	servers := map[string][]string{}
	for k, v := range DefaultServers {
		servers[k] = v
	}
	for k, v := range overrides {
		if len(v) > 0 {
			servers[k] = v
		}
	}
	return &lspManager{
		root:    root,
		servers: servers,
		clients: map[string]*lsp.Client{},
		opened:  map[string]bool{},
	}
}

func (lm *lspManager) client(ctx context.Context, path string) (*lsp.Client, string, error) {
	ext := filepath.Ext(path)
	lang, ok := languages[ext]
	if !ok {
		return nil, "", fmt.Errorf("no LSP support for %s files", ext)
	}
	argv := lm.servers[lang]
	if _, err := exec.LookPath(argv[0]); err != nil {
		return nil, "", fmt.Errorf(
			"no LSP server for %s: install %s or set lsp.%s in .review.yaml",
			ext,
			argv[0],
			lang,
		)
	}
	lm.mu.Lock()
	defer lm.mu.Unlock()
	if c, ok := lm.clients[lang]; ok {
		return c, lang, nil
	}
	c, err := lsp.Start(ctx, argv, lm.root)
	if err != nil {
		return nil, "", err
	}
	lm.clients[lang] = c
	return c, lang, nil
}

func (lm *lspManager) open(c *lsp.Client, path, lang, text string) {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	if lm.opened[path] {
		return
	}
	lm.opened[path] = true
	_ = c.DidOpen(path, lang, text)
}

func (lm *lspManager) close() {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for _, c := range lm.clients {
		_ = c.Shutdown(ctx)
	}
	lm.clients = map[string]*lsp.Client{}
}

func (m *model) codeDir() string {
	if m.review != nil && m.review.Worktree != "" {
		return m.review.Worktree
	}
	return m.repo.Dir
}

func (m *model) manager() *lspManager {
	root := m.codeDir()
	if m.lsp != nil && m.lsp.root == root {
		return m.lsp
	}
	if m.lsp != nil {
		m.lsp.close()
	}
	m.lsp = newLSPManager(root, m.lspServers)
	return m.lsp
}

func (m *model) defaultLSP(kind, file string, line, col int) tea.Cmd {
	mgr, root, parent := m.manager(), m.codeDir(), m.ctx
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, lspTimeout)
		defer cancel()
		abs := file
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(root, file)
		}
		c, lang, err := mgr.client(ctx, abs)
		if err != nil {
			return lspMsg{kind: kind, err: err}
		}
		content, err := os.ReadFile(abs)
		if err != nil {
			return lspMsg{kind: kind, err: err}
		}
		mgr.open(c, abs, lang, string(content))
		lines := strings.Split(string(content), "\n")
		char := 0
		if line-1 < len(lines) {
			char = lsp.UTF16Column(lines[line-1], col, 4)
		}
		if q, ok := strings.CutPrefix(kind, "workspace:"); ok {
			syms, err := c.WorkspaceSymbols(ctx, q)
			return lspMsg{kind: "workspace", locs: symbolLocs(root, abs, syms), err: err}
		}
		switch kind {
		case "symbols":
			syms, err := c.DocumentSymbols(ctx, abs)
			return lspMsg{kind: kind, locs: symbolLocs(root, abs, syms), err: err}
		case "hover":
			h, err := c.Hover(ctx, abs, line-1, char)
			return lspMsg{kind: kind, hover: h, err: err}
		case "references":
			locs, err := c.References(ctx, abs, line-1, char)
			return lspMsg{kind: kind, locs: toLocs(root, locs), err: err}
		case "callers":
			locs, err := c.IncomingCalls(ctx, abs, line-1, char)
			return lspMsg{kind: kind, locs: toLocs(root, locs), err: err}
		default:
			locs, err := c.Locate(ctx, kind, abs, line-1, char)
			return lspMsg{kind: kind, locs: toLocs(root, locs), err: err}
		}
	}
}

func toLocs(root string, locs []lsp.Location) []lspLoc {
	cache := map[string][]string{}
	out := make([]lspLoc, 0, len(locs))
	for _, l := range locs {
		lines, ok := cache[l.Path]
		if !ok {
			data, _ := os.ReadFile(l.Path)
			lines = strings.Split(string(data), "\n")
			cache[l.Path] = lines
		}
		text := ""
		if l.Line < len(lines) {
			text = expandTabs(strings.TrimSpace(lines[l.Line]))
		}
		path := l.Path
		if rel, err := filepath.Rel(root, l.Path); err == nil && !strings.HasPrefix(rel, "..") {
			path = rel
		}
		out = append(out, lspLoc{Path: path, Line: l.Line + 1, Text: text})
	}
	return out
}

func (m *model) defaultPeek(path string) []string {
	abs := path
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(m.codeDir(), path)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return []string{err.Error()}
	}
	return Highlight(path, expandTabs(string(data)))
}

func isIdent(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func wordStart(line string, col int) int {
	rs := []rune(line)
	col = max(0, min(col, len(rs)))
	if col < len(rs) && isIdent(rs[col]) {
		for col > 0 && isIdent(rs[col-1]) {
			col--
		}
		return col
	}
	for i := col; i < len(rs); i++ {
		if isIdent(rs[i]) {
			return i
		}
	}
	for i := min(col, len(rs)-1); i >= 0; i-- {
		if isIdent(rs[i]) {
			return wordStart(line, i)
		}
	}
	return 0
}

func nextWord(line string, col int) int {
	rs := []rune(line)
	i := col
	for i < len(rs) && isIdent(rs[i]) {
		i++
	}
	for i < len(rs) && !isIdent(rs[i]) {
		i++
	}
	if i >= len(rs) {
		return col
	}
	return i
}

func prevWord(line string, col int) int {
	rs := []rune(line)
	i := min(col, len(rs)) - 1
	for i >= 0 && !isIdent(rs[i]) {
		i--
	}
	if i < 0 {
		return col
	}
	for i > 0 && isIdent(rs[i-1]) {
		i--
	}
	return i
}

func wordBounds(line string, col int) (int, int) {
	rs := []rune(line)
	from := wordStart(line, col)
	to := from
	for to < len(rs) && isIdent(rs[to]) {
		to++
	}
	return from, to
}

func (m *model) currentCode() (string, bool) {
	l := m.current()
	if l.Pair && m.useSplit() {
		return l.Right.Plain, l.Right.Line > 0
	}
	if l.Kind != RowCode && l.Kind != RowAdded {
		return "", false
	}
	if l.Plain != "" {
		return l.Plain, true
	}
	return ansi.Strip(l.Text), true
}

func (m *model) lspRequest(kind string) tea.Cmd {
	plain, ok := m.currentCode()
	if !ok {
		m.status = "LSP works on lines of the new code: put the cursor on one"
		return nil
	}
	m.col = wordStart(plain, m.col)
	it := m.current()
	m.lspBusy = kind
	if m.lspDo == nil {
		return nil
	}
	return m.lspDo(kind, it.File, it.Line, m.col)
}

func (m *model) handleLSP(msg lspMsg) {
	m.lspBusy = ""
	if msg.err != nil {
		m.err = msg.err
		return
	}
	name := lspNames[msg.kind]
	empty := len(msg.locs) == 0
	if msg.kind == "hover" {
		empty = strings.TrimSpace(msg.hover) == ""
	}
	if empty {
		m.status = "no " + name + " found"
		return
	}
	if m.popup != nil {
		m.popupStack = append(m.popupStack, m.popup)
	}
	switch {
	case msg.kind == "hover":
		w := max(m.mainWidth()-6, 20)
		m.popup = &popup{
			kind:  "hover",
			title: "hover",
			lines: strings.Split(ansi.Wrap(expandTabs(msg.hover), w, ""), "\n"),
		}
	case len(msg.locs) == 1 && !listKinds[msg.kind]:
		m.openPeek(msg.locs[0])
	default:
		if msg.kind == "symbols" {
			m.markChanged(msg.locs)
		}
		m.popup = &popup{
			kind:  msg.kind,
			title: fmt.Sprintf("%s · %d", name, len(msg.locs)),
			items: msg.locs,
		}
	}
}

func (m *model) peek(path string) []string {
	if m.peekFile != nil {
		return m.peekFile(path)
	}
	return m.defaultPeek(path)
}

func (m *model) openPeek(loc lspLoc) {
	target := max(loc.Line-1, 0)
	m.popup = &popup{
		kind:   "peek",
		title:  fmt.Sprintf("%s:%d", loc.Path, loc.Line),
		lines:  m.peek(loc.Path),
		target: target,
		cursor: target,
		top:    max(target-3, 0),
		loc:    loc,
	}
}

func (m *model) refPreview(p *popup, loc lspLoc, width, rows int) []string {
	if p.files == nil {
		p.files = map[string][]string{}
	}
	lines, ok := p.files[loc.Path]
	if !ok {
		lines = m.peek(loc.Path)
		p.files[loc.Path] = lines
	}
	target := loc.Line - 1
	top := max(0, min(target-rows/2, len(lines)-rows))
	out := codeLines(lines, top, target, rows)
	for i := range out {
		out[i] = ansi.Truncate(out[i], width, "")
	}
	return out
}

func codeLines(lines []string, top, target, rows int) []string {
	var out []string
	for i := top; i < len(lines) && len(out) < rows; i++ {
		num := dimStyle.Render(fmt.Sprintf("%4d │ ", i+1))
		if i == target {
			num = hotStyle.Render(fmt.Sprintf("%4d ▶ ", i+1))
		}
		out = append(out, num+lines[i])
	}
	return out
}

func (m *model) handlePopupKey(msg tea.KeyMsg) tea.Cmd {
	p := m.popup
	isList := p.items != nil
	if p.kind == "peek" {
		if cmd, handled := m.peekKey(p, msg.String()); handled {
			return cmd
		}
	}
	switch msg.String() {
	case "j", "down":
		if isList {
			p.sel = min(p.sel+1, len(p.items)-1)
		} else {
			p.top = min(p.top+1, max(len(p.lines)-1, 0))
		}
	case "k", "up":
		if isList {
			p.sel = max(p.sel-1, 0)
		} else {
			p.top = max(p.top-1, 0)
		}
	case "ctrl+d":
		p.top = min(p.top+10, max(len(p.lines)-1, 0))
	case "ctrl+u":
		p.top = max(p.top-10, 0)
	case "enter":
		if isList && len(p.items) > 0 {
			m.popupStack = append(m.popupStack, p)
			m.openPeek(p.items[p.sel])
		}
	case "e":
		loc := p.loc
		if isList && len(p.items) > 0 {
			loc = p.items[p.sel]
		}
		if loc.Path != "" {
			cmd := editorCmd(m.codeDir(), loc.Path, max(loc.Line, 1))
			return tea.ExecProcess(cmd, func(err error) tea.Msg { return editorDoneMsg{err} })
		}
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		if i := int(msg.String()[0] - '1'); i < len(p.refs) {
			m.popupStack = append(m.popupStack, p)
			m.openPeek(p.refs[i])
		}
	case "esc", "q", "ctrl+o":
		if n := len(m.popupStack); n > 0 {
			m.popup, m.popupStack = m.popupStack[n-1], m.popupStack[:n-1]
		} else {
			m.popup = nil
		}
	}
	return nil
}

func (m *model) popupLines(width, height int) []string {
	p := m.popup
	border := dimStyle.Render("│ ")
	hint := "esc close"
	switch p.kind {
	case "references", "definition", "implementation", "typeDefinition", "callers",
		"symbols", "workspace":
		hint = "j/k select · enter peek · e editor · esc close"
	case "peek":
		hint = "j/k w/b move · gd gr gi gy gc K · e editor · esc back"
	case "detail":
		hint = "j/k or wheel scroll · esc close"
		if len(p.refs) > 0 {
			hint = "1-9 open code · j/k scroll · esc close"
		}
	}
	head := fmt.Sprintf("┌─ %s ", p.title)
	fill := max(width-ansi.StringWidth(head)-ansi.StringWidth(hint)-3, 1)
	head += strings.Repeat("─", fill) + " " + hint
	out := []string{hotStyle.Render(ansi.Truncate(head, width, ""))}
	rows := height - 1
	switch {
	case p.items != nil:
		listW := width - 2
		var code []string
		if width >= minPreviewWidth && len(p.items) > 0 {
			listW = width * 2 / 5
			code = m.refPreview(p, p.items[p.sel], width-listW-5, rows)
		}
		start := max(0, min(p.sel-rows/2, len(p.items)-rows))
		for i := start; i < len(p.items) && len(out) <= rows; i++ {
			it := p.items[i]
			line := fmt.Sprintf("%s:%d  %s", it.Path, it.Line, dimStyle.Render(it.Text))
			if i == p.sel {
				line = cursorStyle.Render("▶ ") + line
			} else {
				line = "  " + line
			}
			out = append(out, border+line)
		}
		for len(out) <= rows {
			out = append(out, border)
		}
		if code != nil {
			for i := 1; i < len(out); i++ {
				right := ""
				if i-1 < len(code) {
					right = code[i-1]
				}
				out[i] = fit(out[i], listW) + dimStyle.Render(" │ ") + right
			}
		}
	case p.kind == "peek":
		p.top = max(0, min(p.top, p.cursor), p.cursor-rows+1)
		for i, l := range codeLines(p.lines, p.top, p.target, rows) {
			if p.top+i == p.cursor {
				from, to := wordBounds(ansi.Strip(p.lines[p.cursor]), p.col)
				l = paint(fit(underline(l, peekGutter+from, peekGutter+to), width-2), cursorTone)
			}
			out = append(out, border+l)
		}
	default:
		for i := p.top; i < len(p.lines) && len(out) <= rows; i++ {
			out = append(out, border+p.lines[i])
		}
	}
	for len(out) <= rows {
		out = append(out, border)
	}
	return out
}

// Styled spans end with a full SGR reset, so the underline is re-armed after each escape.
func underline(s string, from, to int) string {
	return markRange(s, from, to, "\x1b[4m", "\x1b[24m")
}

func markRange(s string, from, to int, on, off string) string {
	if from >= to {
		return s
	}
	var b strings.Builder
	n := 0
	for i := 0; i < len(s); {
		if s[i] == '\x1b' {
			j := i + 1
			if j < len(s) && s[j] == '[' {
				for j++; j < len(s) && (s[j] < 0x40 || s[j] > 0x7e); j++ {
				}
				j++
			}
			j = min(j, len(s))
			b.WriteString(s[i:j])
			if n > from && n < to {
				b.WriteString(on)
			}
			i = j
			continue
		}
		switch n {
		case from:
			b.WriteString(on)
		case to:
			b.WriteString(off)
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		b.WriteRune(r)
		n, i = n+1, i+size
	}
	if from < n && n <= to {
		b.WriteString(off)
	}
	return b.String()
}

const peekGutter = len("1234 | ")

var lspNames = map[string]string{
	"hover":          "hover",
	"definition":     "definitions",
	"references":     "references",
	"implementation": "implementations",
	"typeDefinition": "type definitions",
	"callers":        "callers",
	"symbols":        "symbols",
	"workspace":      "symbols",
}

var listKinds = map[string]bool{
	"references": true, "callers": true, "symbols": true, "workspace": true,
}

var peekG = map[string]string{
	"d": "definition", "r": "references", "i": "implementation",
	"y": "typeDefinition", "c": "callers",
}

func (m *model) peekKey(p *popup, k string) (tea.Cmd, bool) {
	plain := ""
	if p.cursor < len(p.lines) {
		plain = ansi.Strip(p.lines[p.cursor])
	}
	if p.gKey {
		p.gKey = false
		if kind, ok := peekG[k]; ok {
			return m.peekLSP(p, plain, kind), true
		}
		return nil, true
	}
	switch k {
	case "j", "down":
		p.cursor, p.col = min(p.cursor+1, max(len(p.lines)-1, 0)), 0
	case "k", "up":
		p.cursor, p.col = max(p.cursor-1, 0), 0
	case "w":
		p.col = nextWord(plain, p.col)
	case "b":
		p.col = prevWord(plain, p.col)
	case "g":
		p.gKey = true
	case "K":
		return m.peekLSP(p, plain, "hover"), true
	default:
		return nil, false
	}
	return nil, true
}

func (m *model) peekLSP(p *popup, plain, kind string) tea.Cmd {
	if m.lspDo == nil {
		return nil
	}
	p.col = wordStart(plain, p.col)
	m.lspBusy = kind
	return m.lspDo(kind, p.loc.Path, p.cursor+1, p.col)
}

const (
	lspWait      = 30 * time.Second
	lspRefresh   = 15 * time.Second
	lspRefreshes = 8
)

type lspRefreshMsg struct {
	step    string
	attempt int
}

func (m *model) refreshLSPLater(attempt int) tea.Cmd {
	if attempt > lspRefreshes {
		return nil
	}
	step := m.step.ID
	return tea.Tick(lspRefresh, func(time.Time) tea.Msg {
		return lspRefreshMsg{step: step, attempt: attempt}
	})
}

var agentKinds = map[string]bool{"note": true, "spec": true, "hotspot": true}

func symbolLocs(root, file string, syms []lsp.Symbol) []lspLoc {
	out := make([]lspLoc, 0, len(syms))
	for _, s := range syms {
		path := cmp.Or(s.Path, file)
		if rel, err := filepath.Rel(root, path); err == nil && !strings.HasPrefix(rel, "..") {
			path = rel
		}
		text := strings.Repeat("  ", s.Depth) + s.Kind + " " + s.Name
		out = append(out, lspLoc{Path: path, Line: s.Line + 1, End: s.End + 1, Text: text})
	}
	return out
}

func (m *model) markChanged(locs []lspLoc) {
	if m.src == nil {
		return
	}
	for i, l := range locs {
		touches := func(h diff.Hunk) bool {
			return h.NewStart <= max(l.End, l.Line) && l.Line <= h.NewEnd()
		}
		mark := "  "
		fd, err := m.src.FileDiff(l.Path)
		if err == nil && slices.ContainsFunc(fd.Hunks, touches) {
			mark = "● "
		}
		locs[i].Text = mark + l.Text
	}
}

const maxFlow = 6

var callableKinds = map[string]bool{"func": true, "method": true, "constructor": true}

type flowEntry struct {
	name    string
	in, out []string
}

type flowMsg struct {
	step    string
	entries []flowEntry
}

func (m *model) fetchFlow() tea.Cmd {
	mgr, root, parent, step := m.manager(), m.codeDir(), m.ctx, m.step.ID
	changed := map[string][][2]int{}
	for _, h := range m.step.Hunks {
		if _, done := changed[h.File]; done || m.src == nil {
			continue
		}
		fd, err := m.src.FileDiff(h.File)
		if err != nil {
			continue
		}
		for _, dh := range fd.Hunks {
			changed[h.File] = append(changed[h.File], [2]int{dh.NewStart, dh.NewEnd()})
		}
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, lspWait)
		defer cancel()
		var entries []flowEntry
		for file, ranges := range changed {
			abs := filepath.Join(root, file)
			c, lang, err := mgr.client(ctx, abs)
			if err != nil {
				continue
			}
			content, err := os.ReadFile(abs)
			if err != nil {
				continue
			}
			mgr.open(c, abs, lang, string(content))
			syms, err := c.DocumentSymbols(ctx, abs)
			if err != nil {
				continue
			}
			for _, s := range syms {
				touched := slices.ContainsFunc(ranges, func(r [2]int) bool {
					return r[0] <= s.End+1 && s.Line+1 <= r[1]
				})
				if !callableKinds[s.Kind] || !touched || len(entries) >= maxFlow {
					continue
				}
				in, out, err := c.Calls(ctx, abs, s.Line, s.Char)
				if err != nil {
					continue
				}
				e := flowEntry{name: s.Name, in: callNames(root, in), out: callNames(root, out)}
				entries = append(entries, e)
			}
		}
		return flowMsg{step: step, entries: entries}
	}
}

func callNames(root string, calls []lsp.Call) []string {
	var names []string
	for _, c := range calls {
		inside := strings.HasPrefix(c.Path, root+string(filepath.Separator))
		if inside && !slices.Contains(names, c.Name) {
			names = append(names, c.Name)
		}
	}
	return names
}
