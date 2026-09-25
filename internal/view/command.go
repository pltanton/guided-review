package view

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/aplotnikov/guided-review/internal/gitx"
	"github.com/aplotnikov/guided-review/internal/inbox"
)

var commandNames = []string{
	"q", "quit", "help", "noh", "step", "all", "boilerplate", "generated", "file",
	"set", "msg", "skip", "next", "explain", "publish", "edit",
}

var setOptions = []string{
	"split", "nosplit", "plan", "noplan", "mouse", "nomouse", "removed", "noremoved", "context=", "diff=",
}

func (m *model) startCmd(mode rune) {
	m.composing, m.composeKind, m.cmdMode = true, "", mode
	m.input, m.inputPos, m.histIdx = nil, 0, len(m.history)
}

func (m *model) submitCmd() tea.Cmd {
	line := strings.TrimSpace(string(m.input))
	mode := m.cmdMode
	m.composing, m.input, m.cmdMode = false, nil, 0
	if line == "" {
		return nil
	}
	if mode == '/' {
		m.search = line
		m.searchStep(1)
		return nil
	}
	if len(m.history) == 0 || m.history[len(m.history)-1] != line {
		m.history = append(m.history, line)
	}
	return m.execCommand(line)
}

func (m *model) execCommand(line string) tea.Cmd {
	name, arg, _ := strings.Cut(line, " ")
	arg = strings.TrimSpace(arg)
	if n, err := strconv.Atoi(name); err == nil {
		m.gotoLine(n)
		return nil
	}
	switch name {
	case "q", "quit", "qa":
		return tea.Quit
	case "h", "help":
		m.help, m.helpTop = true, 0
		return nil
	case "noh", "nohlsearch":
		m.search = ""
		return nil
	case "step":
		return m.showStepByName(arg)
	case "all", "boilerplate", "generated":
		return m.showStepByName(name)
	case "f", "file":
		m.jumpToFileMatch(arg)
		return nil
	case "set":
		return m.setOption(arg)
	case "msg", "m":
		m.sendMessage(arg)
		return nil
	case "skip":
		if arg == "" {
			m.status = "usage: :skip <reason>"
			return nil
		}
		m.emit(inbox.Event{Kind: inbox.KindSkip, Text: arg})
		return nil
	case "next":
		m.next()
		return nil
	case "explain":
		m.explain()
		return nil
	case "publish":
		m.publish()
		return nil
	case "e", "edit":
		return m.openEditor()
	}
	if m.review != nil && m.stepByID(name) != nil {
		return m.showStep(name)
	}
	for _, a := range m.keys().actions {
		if a.Name == name {
			return a.run(m)
		}
	}
	m.status = "unknown command: " + name + " (h lists actions)"
	return nil
}

func (m *model) showStepByName(name string) tea.Cmd {
	switch name {
	case "all", "boilerplate", "generated":
		name = "~" + name
	case "", "current":
		name = m.review.Current
	}
	if m.review == nil || m.stepByID(name) == nil {
		m.status = "no step " + name
		return nil
	}
	return m.showStep(name)
}

func (m *model) gotoLine(n int) {
	file := m.current().File
	if file == "" {
		for _, it := range m.list {
			if it.File != "" {
				file = it.File
				break
			}
		}
	}
	best := -1
	for i, it := range m.list {
		if it.File != file || it.Note || it.Line == 0 {
			continue
		}
		if it.Line == n {
			best = i
			break
		}
		if it.Line > n && best < 0 {
			best = i
		}
	}
	if best < 0 {
		m.status = fmt.Sprintf("line %d is not shown in %s (o on ⋯ reveals hidden lines)", n, file)
		return
	}
	if m.list[best].Line != n {
		m.status = fmt.Sprintf("line %d is hidden, nearest shown: %d", n, m.list[best].Line)
	}
	m.cursor = best
	m.clamp()
}

func (m *model) jumpToFileMatch(q string) {
	q = strings.ToLower(q)
	for _, f := range m.stepFiles() {
		if strings.Contains(strings.ToLower(f), q) {
			m.jumpToFile(f)
			return
		}
	}
	m.status = "no file matching " + q + " in this step"
}

func (m *model) setOption(arg string) tea.Cmd {
	opt, value, _ := strings.Cut(arg, "=")
	switch opt {
	case "split", "nosplit":
		m.splitView = opt == "split"
		m.relist()
	case "plan", "noplan":
		m.showPlan = opt == "plan"
		m.relist()
	case "removed", "noremoved":
		m.showRemoved = opt == "removed"
		m.relist()
	case "mouse", "nomouse":
		if m.mouse != (opt == "mouse") {
			return m.toggleMouse()
		}
	case "context":
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 {
			m.status = "usage: :set context=N"
			return nil
		}
		m.context = n
		if m.src != nil && m.step != nil {
			m.rebuild(false)
		}
	case "diff":
		if !slices.Contains(gitx.DiffAlgorithms, value) {
			m.status = fmt.Sprintf("diff must be one of %v", gitx.DiffAlgorithms)
			return nil
		}
		m.algo = value
		if m.src != nil {
			m.src = newGitSource(m.ctx, m.repo, m.algo, m.src.base, m.src.head)
			m.unfolded = nil
			m.rebuild(false)
		}
	default:
		m.status = "unknown option: " + arg + " (" + strings.Join(setOptions, " ") + ")"
	}
	return nil
}

func (m *model) sendMessage(text string) {
	if text == "" {
		m.status = "usage: :msg <text>"
		return
	}
	e := inbox.Event{Kind: inbox.KindMessage, Text: text}
	cur := m.current()
	switch {
	case cur.Ref > 0:
		e.Comment, e.File, e.Lines = cur.Ref, cur.File, fmt.Sprint(cur.Line)
	case cur.File != "" && cur.Line > 0:
		e.File, e.Lines = cur.File, fmt.Sprint(cur.Line)
	}
	m.emit(e)
}

func (m *model) complete() {
	line := string(m.input)
	var prefix string
	var candidates []string
	if rest, ok := strings.CutPrefix(line, "set "); ok {
		prefix = "set "
		for _, o := range setOptions {
			if strings.HasPrefix(o, rest) {
				candidates = append(candidates, o)
			}
		}
	} else if !strings.Contains(line, " ") {
		names := slices.Clone(commandNames)
		for _, a := range m.keys().actions {
			names = append(names, a.Name)
		}
		if m.review != nil {
			names = append(names, m.stepIDs()...)
		}
		for _, n := range names {
			if strings.HasPrefix(n, line) && !slices.Contains(candidates, n) {
				candidates = append(candidates, n)
			}
		}
	}
	switch len(candidates) {
	case 0:
		m.status = "no completion"
		return
	case 1:
		m.input = []rune(prefix + candidates[0])
	default:
		common := candidates[0]
		for _, c := range candidates[1:] {
			for !strings.HasPrefix(c, common) {
				common = common[:len(common)-1]
			}
		}
		m.input = []rune(prefix + common)
		m.status = strings.Join(candidates[:min(len(candidates), 8)], "  ")
	}
	m.inputPos = len(m.input)
}

func (m *model) historyMove(d int) {
	if len(m.history) == 0 {
		return
	}
	m.histIdx = max(0, min(m.histIdx+d, len(m.history)))
	if m.histIdx == len(m.history) {
		m.input = nil
	} else {
		m.input = []rune(m.history[m.histIdx])
	}
	m.inputPos = len(m.input)
}

func (m *model) rowText(i int) string {
	strip := func(plain, text string) string {
		if plain != "" {
			return plain
		}
		return ansi.Strip(text)
	}
	if m.useSplit() {
		r := m.split[i]
		if r.Full != nil {
			return strip(r.Full.Plain, r.Full.Text)
		}
		return strip(r.Right.Plain, r.Right.Text) + "\n" + strip(r.Left.Plain, r.Left.Text)
	}
	r := m.disp[i]
	return strip(r.Plain, r.Text)
}

func (m *model) matches() []int {
	q := m.search
	fold := !strings.ContainsFunc(q, unicode.IsUpper)
	if fold {
		q = strings.ToLower(q)
	}
	var out []int
	for i := range m.list {
		text := m.rowText(i)
		if fold {
			text = strings.ToLower(text)
		}
		if strings.Contains(text, q) {
			out = append(out, i)
		}
	}
	return out
}

func (m *model) searchStep(dir int) {
	ms := m.matches()
	if len(ms) == 0 {
		m.status = "no match: " + m.search
		return
	}
	pick := -1
	if dir > 0 {
		for k, i := range ms {
			if i > m.cursor {
				pick = k
				break
			}
		}
		if pick < 0 {
			pick = 0
		}
	} else {
		for k := len(ms) - 1; k >= 0; k-- {
			if ms[k] < m.cursor {
				pick = k
				break
			}
		}
		if pick < 0 {
			pick = len(ms) - 1
		}
	}
	m.cursor = ms[pick]
	m.clamp()
	m.status = fmt.Sprintf("match %d/%d · /%s", pick+1, len(ms), m.search)
}
