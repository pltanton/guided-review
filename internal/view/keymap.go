package view

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/pltanton/guided-review/internal/gitx"
	"github.com/pltanton/guided-review/internal/inbox"
)

type Action struct {
	Name  string
	Group string
	Desc  string
	Keys  []string
	run   func(*model) tea.Cmd
}

const finishGroup = "finish preview"

var (
	groups         = []string{"navigate", "diff", "lsp", "review", "view", finishGroup}
	previewActions = []string{"finish", "edit-comment", "delete-comment", "message"}
	essentials     = []string{
		"down", "up", "next-hunk", "prev-hunk", "message", "ask", "next", "skip",
		"details", "plan", "finish", "command", "quit",
	}
)

func DefaultActions() []Action {
	do := func(f func(*model)) func(*model) tea.Cmd {
		return func(m *model) tea.Cmd { f(m); return nil }
	}
	jump := func(dir int, match func(line) bool) func(*model) tea.Cmd {
		return do(func(m *model) { m.jump(dir, match) })
	}
	hunk := func(l line) bool { return l.HunkStart }
	file := func(l line) bool { return l.Kind == RowFile }
	k := func(keys ...string) []string { return keys }
	const nav, dif, lsp, rev, vw = "navigate", "diff", "lsp", "review", "view"
	return []Action{
		{Name: "down", Group: nav, Desc: "move down", Keys: k("j", "down"),
			run: do(func(m *model) { m.move(1) })},
		{Name: "up", Group: nav, Desc: "move up", Keys: k("k", "up"),
			run: do(func(m *model) { m.move(-1) })},
		{Name: "half-down", Group: nav, Desc: "half a page down", Keys: k("ctrl+d"),
			run: do(func(m *model) { m.move(m.bodyHeight() / 2) })},
		{Name: "half-up", Group: nav, Desc: "half a page up", Keys: k("ctrl+u"),
			run: do(func(m *model) { m.move(-m.bodyHeight() / 2) })},
		{Name: "top", Group: nav, Desc: "first line", Keys: k("g g", "home"),
			run: do(func(m *model) { m.cursor = 0; m.clamp() })},
		{Name: "bottom", Group: nav, Desc: "last line", Keys: k("G", "end"),
			run: do(func(m *model) { m.cursor = len(m.lines) - 1; m.clamp() })},
		{Name: "next-hunk", Group: nav, Desc: "next change", Keys: k("]"), run: jump(1, hunk)},
		{
			Name:  "prev-hunk",
			Group: nav,
			Desc:  "previous change",
			Keys:  k("["),
			run:   jump(-1, hunk),
		},
		{
			Name:  "next-note",
			Group: nav,
			Desc:  "next search match, else next annotation",
			Keys:  k("n"),
			run:   do(func(m *model) { m.nextNote(1) }),
		},
		{Name: "prev-note", Group: nav, Desc: "previous match or annotation", Keys: k("N"),
			run: do(func(m *model) { m.nextNote(-1) })},
		{Name: "search", Group: nav, Desc: "search this step (n/N walk, esc clears)", Keys: k("/"),
			run: do(func(m *model) { m.startCmd('/') })},
		{Name: "next-file", Group: nav, Desc: "next file", Keys: k("}"), run: jump(1, file)},
		{Name: "prev-file", Group: nav, Desc: "previous file", Keys: k("{"), run: jump(-1, file)},
		{Name: "files", Group: nav, Desc: "focus the files panel", Keys: k("f"),
			run: do((*model).focusFilesPanel)},
		{Name: "steps", Group: nav, Desc: "focus the plan panel: fold chapters, preview steps",
			Keys: k("ctrl+p"), run: do((*model).focusPlanPanel)},
		{
			Name:  "prev-step",
			Group: nav,
			Desc:  "look at the previous step (progress stays)",
			Keys:  k("H"),
			run:   func(m *model) tea.Cmd { return m.shiftStep(-1) },
		},
		{Name: "next-step-view", Group: nav, Desc: "look at the next step, then extra views",
			Keys: k("L"), run: func(m *model) tea.Cmd { return m.shiftStep(1) }},
		{
			Name:  "back",
			Group: nav,
			Desc:  "clear selection / search / chat, back to the current step",
			Keys:  k("esc"),
			run:   (*model).back,
		},
		{Name: "word-next", Group: nav, Desc: "next symbol in the line", Keys: k("w"),
			run: do((*model).wordNext)},
		{Name: "word-prev", Group: nav, Desc: "previous symbol in the line", Keys: k("b"),
			run: do((*model).wordPrev)},
		{Name: "scroll-left", Group: nav, Desc: "scroll long lines left (no wrap)", Keys: k("h"),
			run: do(func(m *model) { m.scrollSideways(-hscrollStep) })},
		{Name: "scroll-right", Group: nav, Desc: "scroll long lines right (no wrap)", Keys: k("l"),
			run: do(func(m *model) { m.scrollSideways(hscrollStep) })},

		{Name: "open", Group: dif, Desc: "open ⋯ hidden lines or a ▸ folded block; fold a note",
			Keys: k("o"), run: do((*model).toggleFold)},
		{
			Name:  "all-removed",
			Group: dif,
			Desc:  "show every removed line / fold again",
			Keys:  k("O"),
			run:   do((*model).toggleRemoved),
		},
		{Name: "more-context", Group: dif, Desc: "more context around changes", Keys: k("tab"),
			run: do((*model).moreContext)},
		{Name: "reset-context", Group: dif, Desc: "default context", Keys: k("shift+tab"),
			run: do((*model).resetContext)},
		{Name: "split", Group: dif, Desc: "split / unified", Keys: k("s"),
			run: do((*model).toggleSplit)},
		{Name: "diff-algorithm", Group: dif, Desc: "next diff algorithm", Keys: k("d"),
			run: do((*model).nextAlgorithm)},

		{Name: "definition", Group: lsp, Desc: "go to definition (peek)", Keys: k("g d"),
			run: func(m *model) tea.Cmd { return m.lspRequest("definition") }},
		{Name: "references", Group: lsp, Desc: "list references", Keys: k("g r"),
			run: func(m *model) tea.Cmd { return m.lspRequest("references") }},
		{Name: "implementation", Group: lsp, Desc: "go to implementations", Keys: k("g i"),
			run: func(m *model) tea.Cmd { return m.lspRequest("implementation") }},
		{Name: "type-definition", Group: lsp, Desc: "go to the type's definition", Keys: k("g y"),
			run: func(m *model) tea.Cmd { return m.lspRequest("typeDefinition") }},
		{Name: "callers", Group: lsp, Desc: "who calls this (incoming calls)", Keys: k("g c"),
			run: func(m *model) tea.Cmd { return m.lspRequest("callers") }},
		{Name: "symbols", Group: lsp, Desc: "symbols of this file (● changed)", Keys: k("g s"),
			run: func(m *model) tea.Cmd { return m.lspRequest("symbols") }},
		{Name: "hover", Group: lsp, Desc: "type and docs", Keys: k("K"),
			run: func(m *model) tea.Cmd { return m.lspRequest("hover") }},

		{Name: "next", Group: rev, Desc: "done with this step, go on", Keys: k(">"),
			run: do((*model).next)},
		{Name: "message", Group: rev, Desc: "message the agent (line attached); opens ⋯ / ▸",
			Keys: k("c", "enter"), run: do((*model).messageOrOpen)},
		{Name: "message-general", Group: rev, Desc: "message the agent without a line",
			Keys: k("C"), run: do(func(m *model) {
				m.startCompose(inbox.KindMessage)
				m.anchorFile, m.anchorLines, m.composeRef = "", "", 0
			})},
		{Name: "ask", Group: rev, Desc: "ask about the line / selection; enter alone: explain it",
			Keys: k("A"), run: do(func(m *model) { m.startCompose(inbox.KindAsk) })},
		{Name: "details", Group: rev, Desc: "details behind the agent's note under the cursor",
			Keys: k("i"), run: do((*model).noteDetails)},
		{Name: "yank", Group: rev, Desc: "copy the selection or the line to the clipboard",
			Keys: k("y"), run: do((*model).yank)},
		{Name: "select", Group: rev, Desc: "select lines", Keys: k("v"),
			run: do(func(m *model) { m.visual, m.anchor = !m.visual, m.cursor })},
		{Name: "skip", Group: rev, Desc: "skip the step with a reason", Keys: k("S"),
			run: do(func(m *model) { m.startCompose(inbox.KindSkip) })},
		{Name: "edit-comment", Group: rev, Desc: "edit the comment under the cursor", Keys: k("E"),
			run: do((*model).startEdit)},
		{Name: "delete-comment", Group: rev, Desc: "delete the comment under the cursor (twice)",
			Keys: k("D"), run: do((*model).deleteComment)},
		{Name: "replies", Group: rev, Desc: "your MR threads: answers, resolve or keep open",
			Keys: k("R"), run: do((*model).openThreads)},
		{Name: "finish", Group: rev, Desc: "finish: preview, then hand to the agent",
			Keys: k("P"), run: (*model).finish},
		{Name: "editor", Group: rev, Desc: "open $EDITOR at the line", Keys: k("e"),
			run: (*model).openEditor},

		{Name: "wrap", Group: vw, Desc: "wrap long lines / cut them and scroll sideways",
			Keys: k("W"), run: do(func(m *model) { m.setWrap(m.nowrap) })},
		{Name: "plan", Group: vw, Desc: "show / hide the plan panel", Keys: k("p"),
			run: do(func(m *model) { m.showPlan = !m.showPlan; m.relist() })},
		{Name: "mouse", Group: vw, Desc: "mouse capture on / off", Keys: k("m"),
			run: (*model).toggleMouse},
		{Name: "agent", Group: vw, Desc: "switch to the agent's pane", Keys: k("a"),
			run: do((*model).focusAgent)},
		{Name: "chat", Group: vw, Desc: "select in the chat: j/k move, v select, y copy, esc back",
			Keys: k("t"), run: do((*model).focusChat)},
		{Name: "chat-up", Group: vw, Desc: "scroll the chat up", Keys: k("ctrl+y"),
			run: do(func(m *model) { m.chatTop += wheelStep })},
		{Name: "chat-down", Group: vw, Desc: "scroll the chat down", Keys: k("ctrl+e"),
			run: do(func(m *model) { m.chatTop = max(m.chatTop-wheelStep, 0) })},
		{
			Name:  "command",
			Group: vw,
			Desc:  "command line (:42, :s3, :f name, any action)",
			Keys:  k(":"),
			run:   do(func(m *model) { m.startCmd(':') }),
		},
		{Name: "help", Group: vw, Desc: "this help", Keys: k("?", "f1"),
			run: do(func(m *model) { m.help, m.helpAll, m.helpTop = true, false, 0 })},
		{Name: "interrupt", Group: rev, Desc: "stop the agent's current work and add to your question",
			Keys: k("ctrl+c"), run: do((*model).interrupt)},
		{Name: "quit", Group: vw, Desc: "quit the viewer", Keys: k("q"),
			run: func(*model) tea.Cmd { return tea.Quit }},

		{Name: "verdict", Group: finishGroup, Desc: "verdict: approve → changes → blocked",
			Keys: k("v"), run: do((*model).cycleVerdict)},
		{Name: "approve", Group: finishGroup, Desc: "approve the MR on publishing, or not",
			Keys: k("a"), run: do((*model).toggleApprove)},
		{Name: "severity", Group: finishGroup, Desc: "next severity of the selected comment",
			Keys: k("s"), run: do(func(m *model) { m.cycleSeverity(m.previewSelected()) })},
	}
}

func (m *model) nextNote(dir int) {
	if m.search != "" {
		m.searchStep(dir)
		return
	}
	m.jump(dir, func(l line) bool { return l.NoteHead })
}

func (m *model) messageOrOpen() {
	if cur := m.current(); cur.GapTo > 0 || cur.Kind == RowFold {
		m.toggleFold()
		return
	}
	m.startCompose(inbox.KindMessage)
}

func (m *model) focusAgent() {
	if err := focusAgent(m.returnPane); err != nil {
		m.err = err
	}
}

type keymap struct {
	actions  []Action
	byKey    map[string]int
	preview  map[string]int
	prefixes map[string]bool
}

func newKeymap(overrides map[string][]string) (*keymap, error) {
	km := &keymap{
		actions: DefaultActions(), byKey: map[string]int{}, preview: map[string]int{},
		prefixes: map[string]bool{},
	}
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
	bind := func(byKey map[string]int, i int, k string) bool {
		if j, taken := byKey[k]; taken {
			conflicts = append(
				conflicts,
				fmt.Sprintf("%q: %s and %s", k, km.actions[j].Name, km.actions[i].Name),
			)
			return false
		}
		byKey[k] = i
		return true
	}
	for i, a := range km.actions {
		for _, k := range a.Keys {
			if a.Group == finishGroup || slices.Contains(previewActions, a.Name) {
				bind(km.preview, i, k)
			}
			if a.Group == finishGroup || !bind(km.byKey, i, k) {
				continue
			}
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

func (km *keymap) name(k string) string {
	if i, ok := km.byKey[k]; ok {
		return km.actions[i].Name
	}
	return ""
}

func (km *keymap) previewKey(name, k string) bool {
	i, ok := km.preview[k]
	return ok && km.actions[i].Name == name
}

func (m *model) keys() *keymap {
	if m.km == nil {
		m.km, _ = newKeymap(nil)
	}
	return m.km
}

func (m *model) dispatch(k string) tea.Cmd {
	km := m.keys()
	if m.pendingKey == "" && len(k) == 1 && k >= "0" && k <= "9" && (k != "0" || m.count != "") {
		m.count += k
		return nil
	}
	if m.pendingKey != "" {
		seq := m.pendingKey + " " + k
		m.pendingKey = ""
		if i, ok := km.byKey[seq]; ok {
			return m.runCounted(km.actions[i])
		}
	}
	if km.prefixes[k] {
		m.pendingKey = k
		return nil
	}
	if i, ok := km.byKey[k]; ok {
		return m.runCounted(km.actions[i])
	}
	m.count = ""
	return nil
}

func (m *model) runCounted(a Action) tea.Cmd {
	n, _ := strconv.Atoi(m.count)
	m.count = ""
	switch {
	case n > 0 && (a.Name == "top" || a.Name == "bottom"):
		m.gotoLine(n)
		return nil
	case n > 1 && a.Group == "navigate":
		for range min(n, len(m.lines)) - 1 {
			a.run(m)
		}
	}
	return a.run(m)
}

func (m *model) focusFilesPanel() {
	files := m.stepFiles()
	if len(files) == 0 {
		return
	}
	m.showPlan, m.focusFiles, m.focusPlan = true, true, false
	m.fileCursor = max(0, slices.Index(files, m.current().File))
	m.relist()
}

func (m *model) back() tea.Cmd {
	switch {
	case m.visual:
		m.visual = false
	case m.search != "":
		m.search = ""
	case m.viewStep != "":
		return m.showStep(m.review.Current)
	}
	return nil
}

func (m *model) wordNext() {
	if plain, ok := m.currentCode(); ok {
		m.col = nextWord(plain, wordStart(plain, m.col))
		m.followCol()
	}
}

func (m *model) wordPrev() {
	if plain, ok := m.currentCode(); ok {
		m.col = prevWord(plain, wordStart(plain, m.col))
		m.followCol()
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
		m.context = m.baseCtx
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
	m.setAlgorithm(gitx.DiffAlgorithms[i])
}

func (m *model) setAlgorithm(algo string) {
	m.algo, m.status = algo, "diff: "+algo
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
	if !m.helpAll {
		out := []string{boldStyle.Render("essentials"), ""}
		for _, name := range essentials {
			for _, a := range km.actions {
				if a.Name == name && len(a.Keys) > 0 {
					out = append(out, helpRow(a))
				}
			}
		}
		return out
	}
	var blocks [][]string
	total := 0
	for _, g := range groups {
		block := []string{boldStyle.Render(g)}
		for _, a := range km.actions {
			if a.Group != g {
				continue
			}
			block = append(block, helpRow(a))
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

func helpRow(a Action) string {
	keys := make([]string, len(a.Keys))
	for i, k := range a.Keys {
		keys[i] = strings.ReplaceAll(k, " ", "")
	}
	return cursorStyle.Render(fmt.Sprintf("  %-11s", strings.Join(keys, " "))) + " " + a.Desc
}

func (m *model) handleHelpKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "j", "down":
		m.helpTop++
	case "k", "up":
		m.helpTop = max(m.helpTop-1, 0)
	case "?", "f1":
		m.helpAll, m.helpTop = !m.helpAll, 0
	default:
		m.help = false
	}
	return nil
}

func CheckKeys(overrides map[string][]string) error {
	_, err := newKeymap(overrides)
	return err
}
