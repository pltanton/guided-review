package view

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/aplotnikov/guided-review/internal/gitx"
	"github.com/aplotnikov/guided-review/internal/inbox"
)

type Action struct {
	Name  string
	Group string
	Desc  string
	Keys  []string
	run   func(*model) tea.Cmd
}

var groups = []string{"navigate", "diff", "lsp", "review", "view"}

func DefaultActions() []Action {
	do := func(f func(*model)) func(*model) tea.Cmd {
		return func(m *model) tea.Cmd { f(m); return nil }
	}
	jump := func(dir int, match func(item) bool) func(*model) tea.Cmd {
		return do(func(m *model) { m.jump(dir, match) })
	}
	return []Action{
		{"down", "navigate", "move down", []string{"j", "down"}, do(func(m *model) { m.move(1) })},
		{"up", "navigate", "move up", []string{"k", "up"}, do(func(m *model) { m.move(-1) })},
		{"half-down", "navigate", "half a page down", []string{"ctrl+d"}, do(func(m *model) { m.move(m.bodyHeight() / 2) })},
		{"half-up", "navigate", "half a page up", []string{"ctrl+u"}, do(func(m *model) { m.move(-m.bodyHeight() / 2) })},
		{"top", "navigate", "first line", []string{"g g", "home"}, do(func(m *model) { m.cursor = 0; m.clamp() })},
		{"bottom", "navigate", "last line", []string{"G", "end"}, do(func(m *model) { m.cursor = len(m.list) - 1; m.clamp() })},
		{"next-hunk", "navigate", "next change", []string{"]"}, jump(1, func(it item) bool { return it.HunkStart })},
		{"prev-hunk", "navigate", "previous change", []string{"["}, jump(-1, func(it item) bool { return it.HunkStart })},
		{"next-note", "navigate", "next search match, else next annotation", []string{"n"}, do(func(m *model) {
			if m.search != "" {
				m.searchStep(1)
				return
			}
			m.jump(1, func(it item) bool { return it.Note })
		})},
		{"prev-note", "navigate", "previous search match, else previous annotation", []string{"N"}, do(func(m *model) {
			if m.search != "" {
				m.searchStep(-1)
				return
			}
			m.jump(-1, func(it item) bool { return it.Note })
		})},
		{"search", "navigate", "search in this step (n/N next/prev, esc clears)", []string{"/"}, do(func(m *model) { m.startCmd('/') })},
		{"next-file", "navigate", "next file", []string{"}"}, jump(1, func(it item) bool { return it.FileHead })},
		{"prev-file", "navigate", "previous file", []string{"{"}, jump(-1, func(it item) bool { return it.FileHead })},
		{"files", "navigate", "focus the files panel", []string{"f"}, do((*model).focusFilesPanel)},
		{"prev-step", "navigate", "look at the previous step (progress stays)", []string{"H"}, func(m *model) tea.Cmd { return m.shiftStep(-1) }},
		{"next-step-view", "navigate", "look at the next step, then boilerplate / generated / all", []string{"L"}, func(m *model) tea.Cmd { return m.shiftStep(1) }},
		{"back", "navigate", "clear the selection, back to the current step", []string{"esc"}, (*model).back},
		{"word-next", "navigate", "next symbol in the line", []string{"w"}, do((*model).wordNext)},
		{"word-prev", "navigate", "previous symbol in the line", []string{"b"}, do((*model).wordPrev)},

		{"open", "diff", "open ⋯ hidden lines or a ▸ folded block", []string{"o"}, do((*model).toggleFold)},
		{"all-removed", "diff", "show every removed line / fold again", []string{"O"}, do((*model).toggleRemoved)},
		{"more-context", "diff", "more context around changes", []string{"tab"}, do((*model).moreContext)},
		{"reset-context", "diff", "default context", []string{"shift+tab"}, do((*model).resetContext)},
		{"split", "diff", "split / unified", []string{"s"}, do((*model).toggleSplit)},
		{"diff-algorithm", "diff", "next diff algorithm", []string{"d"}, do((*model).nextAlgorithm)},

		{"definition", "lsp", "go to definition (peek)", []string{"g d"}, func(m *model) tea.Cmd { return m.lspRequest("definition") }},
		{"references", "lsp", "list references", []string{"g r"}, func(m *model) tea.Cmd { return m.lspRequest("references") }},
		{"hover", "lsp", "type and docs", []string{"K"}, func(m *model) tea.Cmd { return m.lspRequest("hover") }},

		{"next", "review", "done with this step, go on", []string{">"}, do((*model).next)},
		{"message", "review", "message the agent (cursor line attached); opens ⋯ / ▸ rows", []string{"c", "enter"}, do(func(m *model) {
			if cur := m.current(); cur.Gap[1] > 0 || cur.Fold != "" && m.cursor < len(m.disp) && m.disp[m.cursor].Kind == RowFold {
				m.toggleFold()
				return
			}
			m.startCompose(inbox.KindMessage)
		})},
		{"explain", "review", "ask the agent to explain the line / selection", []string{"?"}, do((*model).explain)},
		{"select", "review", "select lines", []string{"v"}, do(func(m *model) { m.visual, m.anchor = !m.visual, m.cursor })},
		{"skip", "review", "skip the step with a reason", []string{"S"}, do(func(m *model) { m.startCompose(inbox.KindSkip) })},
		{"edit-comment", "review", "edit the comment under the cursor", []string{"E"}, do((*model).startEdit)},
		{"publish", "review", "preview, then publish to the MR", []string{"P"}, do((*model).publish)},
		{"editor", "review", "open $EDITOR at the line", []string{"e"}, (*model).openEditor},

		{"plan", "view", "show / hide the plan panel", []string{"p"}, do(func(m *model) { m.showPlan = !m.showPlan; m.relist() })},
		{"mouse", "view", "mouse capture on / off", []string{"m"}, (*model).toggleMouse},
		{"agent", "view", "switch to the agent's pane", []string{"a"}, do(func(m *model) {
			if err := focusAgent(m.returnPane); err != nil {
				m.err = err
			}
		})},
		{"chat", "view", "chat size: small → half → full screen", []string{"t"}, do(func(m *model) {
			m.chatSize, m.chatTop = (m.chatSize+1)%3, 0
			m.clamp()
		})},
		{"command", "view", "command line (:42, :s3, :set split, :msg …, any action)", []string{":"}, do(func(m *model) { m.startCmd(':') })},
		{"help", "view", "this help", []string{"h", "f1"}, do(func(m *model) { m.help, m.helpTop = true, 0 })},
		{"quit", "view", "quit the viewer", []string{"q", "ctrl+c"}, func(*model) tea.Cmd { return tea.Quit }},
	}
}

type keymap struct {
	actions  []Action
	byKey    map[string]int
	prefixes map[string]bool
}

func newKeymap(overrides map[string][]string) (*keymap, error) {
	km := &keymap{actions: DefaultActions(), byKey: map[string]int{}, prefixes: map[string]bool{}}
	var unknown []string
	for name, keys := range overrides {
		i := slices.IndexFunc(km.actions, func(a Action) bool { return a.Name == name })
		if i < 0 {
			unknown = append(unknown, name)
			continue
		}
		km.actions[i].Keys = keys
	}
	var conflicts []string
	for i, a := range km.actions {
		for _, k := range a.Keys {
			if j, taken := km.byKey[k]; taken {
				conflicts = append(conflicts, fmt.Sprintf("%q: %s and %s", k, km.actions[j].Name, a.Name))
				continue
			}
			km.byKey[k] = i
			if first, _, seq := strings.Cut(k, " "); seq {
				km.prefixes[first] = true
			}
		}
	}
	sort.Strings(unknown)
	var errs []string
	if len(unknown) > 0 {
		errs = append(errs, "unknown actions: "+strings.Join(unknown, ", "))
	}
	if len(conflicts) > 0 {
		errs = append(errs, "key conflicts: "+strings.Join(conflicts, "; "))
	}
	if len(errs) > 0 {
		return km, fmt.Errorf("keys: %s", strings.Join(errs, "; "))
	}
	return km, nil
}

func (km *keymap) key(name string) string {
	for _, a := range km.actions {
		if a.Name == name && len(a.Keys) > 0 {
			return strings.ReplaceAll(a.Keys[0], " ", "")
		}
	}
	return ""
}

func (m *model) keys() *keymap {
	if m.km == nil {
		m.km, _ = newKeymap(nil)
	}
	return m.km
}

func (m *model) dispatch(k string) tea.Cmd {
	km := m.keys()
	if m.pendingKey != "" {
		seq := m.pendingKey + " " + k
		m.pendingKey = ""
		if i, ok := km.byKey[seq]; ok {
			return km.actions[i].run(m)
		}
	}
	if km.prefixes[k] {
		m.pendingKey = k
		return nil
	}
	if i, ok := km.byKey[k]; ok {
		return km.actions[i].run(m)
	}
	return nil
}

func (m *model) focusFilesPanel() {
	files := m.stepFiles()
	if len(files) == 0 {
		return
	}
	m.showPlan, m.focusFiles = true, true
	m.fileCursor = max(0, slices.Index(files, m.current().File))
	m.relist()
}

func (m *model) back() tea.Cmd {
	switch {
	case m.visual:
		m.visual = false
	case m.search != "":
		m.search = ""
	case m.chatSize > 0:
		m.chatSize, m.chatTop = 0, 0
	case m.viewStep != "":
		return m.showStep(m.review.Current)
	}
	return nil
}

func (m *model) wordNext() {
	if plain, ok := m.currentCode(); ok {
		m.col = nextWord(plain, wordStart(plain, m.col))
	}
}

func (m *model) wordPrev() {
	if plain, ok := m.currentCode(); ok {
		m.col = prevWord(plain, wordStart(plain, m.col))
	}
}

func (m *model) toggleRemoved() {
	if !m.showRemoved && !m.hasFolds() {
		m.status = "no removed lines are folded in this step"
		return
	}
	m.showRemoved = !m.showRemoved
	m.relist()
	m.status = "folding large removed blocks"
	if m.showRemoved {
		m.status = "showing every removed line"
	}
}

func (m *model) moreContext() {
	if m.step != nil {
		m.context += 10
		m.rebuild(false)
	}
}

func (m *model) resetContext() {
	if m.step != nil {
		m.context = m.baseContext()
		m.rebuild(false)
	}
}

func (m *model) toggleSplit() {
	m.splitView = !m.splitView
	m.relist()
	if m.splitView && !m.useSplit() {
		m.status = "too narrow for split: widen the pane or hide the plan"
	}
}

func (m *model) nextAlgorithm() {
	i := (slices.Index(gitx.DiffAlgorithms, m.algo) + 1) % len(gitx.DiffAlgorithms)
	m.algo = gitx.DiffAlgorithms[i]
	m.status = "diff: " + m.algo
	if m.src != nil {
		m.src = newGitSource(m.ctx, m.repo, m.algo, m.src.base, m.src.head)
		m.unfolded = nil
		m.rebuild(false)
	}
}

func (m *model) toggleMouse() tea.Cmd {
	m.mouse = !m.mouse
	if m.mouse {
		return tea.EnableMouseCellMotion
	}
	return tea.DisableMouse
}

func (m *model) helpLines(width int) []string {
	km := m.keys()
	var blocks [][]string
	total := 0
	for _, g := range groups {
		block := []string{boldStyle.Render(g)}
		for _, a := range km.actions {
			if a.Group != g {
				continue
			}
			keys := make([]string, len(a.Keys))
			for i, k := range a.Keys {
				keys[i] = strings.ReplaceAll(k, " ", "")
			}
			block = append(block, cursorStyle.Render(fmt.Sprintf("  %-11s", strings.Join(keys, " ")))+" "+a.Desc)
		}
		blocks = append(blocks, block)
		total += len(block) + 1
	}
	cols := 1
	switch {
	case width >= 150:
		cols = 3
	case width >= 96:
		cols = 2
	}
	target := (total + cols - 1) / cols
	columns := [][]string{nil}
	for _, block := range blocks {
		cur := &columns[len(columns)-1]
		if len(*cur) > 0 && len(*cur)+len(block) > target && len(columns) < cols {
			columns = append(columns, nil)
			cur = &columns[len(columns)-1]
		}
		if len(*cur) > 0 {
			*cur = append(*cur, "")
		}
		*cur = append(*cur, block...)
	}
	rows := 0
	for _, c := range columns {
		rows = max(rows, len(c))
	}
	colWidth := width / cols
	out := make([]string, rows)
	for _, c := range columns {
		for r := range rows {
			line := ""
			if r < len(c) {
				line = c[r]
			}
			out[r] += fit(line, colWidth-1) + " "
		}
	}
	return out
}

func (m *model) handleHelpKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "j", "down":
		m.helpTop++
	case "k", "up":
		m.helpTop = max(m.helpTop-1, 0)
	default:
		m.help = false
	}
	return nil
}

func CheckKeys(overrides map[string][]string) error {
	_, err := newKeymap(overrides)
	return err
}
