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

var commandNames = []string{"q", "all", "boilerplate", "generated", "f", "set", "msg", "skip"}

var setOptions = []string{"context=", "diff="}

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
	case "q":
		return tea.Quit
	case "all", "boilerplate", "generated":
		name = "~" + name
	case "f":
		m.jumpToFileMatch(arg)
		return nil
	case "set":
		m.setOption(arg)
		return nil
	case "msg", "skip":
		kind := inbox.KindMessage
		if name == "skip" {
			kind = inbox.KindSkip
		}
		if arg == "" {
			m.status = "usage: :" + name + " <text>"
			return nil
		}
		e := inbox.Event{Kind: kind, Text: arg}
		if kind == inbox.KindMessage {
			e.File, e.Lines, e.Comment = m.anchorAt()
		}
		m.emit(e)
		return nil
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

func (m *model) gotoLine(n int) {
	file := m.current().File
	if file == "" {
		for _, it := range m.lines {
			if it.File != "" {
				file = it.File
				break
			}
		}
	}
	best := -1
	for i, it := range m.lines {
		if it.File != file || it.NoteHead || it.Line == 0 {
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
	if m.lines[best].Line != n {
		m.status = fmt.Sprintf("line %d is hidden, nearest shown: %d", n, m.lines[best].Line)
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

func (m *model) setOption(arg string) {
	opt, value, _ := strings.Cut(arg, "=")
	switch opt {
	case "context":
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 {
			m.status = "usage: :set context=N"
			return
		}
		m.context = n
	case "diff":
		if !slices.Contains(gitx.DiffAlgorithms, value) {
			m.status = fmt.Sprintf("diff must be one of %v", gitx.DiffAlgorithms)
			return
		}
		m.setAlgorithm(value)
		return
	default:
		m.status = "usage: :set context=N | diff=ALGORITHM"
		return
	}
	if m.src != nil && m.step != nil {
		m.rebuild(false)
	}
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
	l := m.lines[i]
	if l.Pair && m.useSplit() {
		return strip(l.Right.Plain, l.Right.Text) + "\n" + strip(l.Left.Plain, l.Left.Text)
	}
	return strip(l.Plain, l.Text)
}

func (m *model) matches() []int {
	q := m.search
	fold := !strings.ContainsFunc(q, unicode.IsUpper)
	if fold {
		q = strings.ToLower(q)
	}
	var out []int
	for i := range m.lines {
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
