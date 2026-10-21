package view

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/pltanton/guided-review/internal/lsp"
)

const (
	hereDelay      = 250 * time.Millisecond
	maxHereCallers = 3
	maxHereHover   = 2
)

var callableKinds = map[string]bool{"func": true, "method": true, "constructor": true}

type hereReq struct {
	kind       string
	file       string
	line, char int
}

type hereMsg struct {
	req     hereReq
	syms    []lsp.Symbol
	hover   string
	callers []string
}

type hereTickMsg struct{ seq int }

type hereState struct {
	file      string
	syms      []lsp.Symbol
	tries     int
	key       string
	seq       int
	hover     string
	hoverKey  string
	callers   []string
	callersOf string
}

func (m *model) defaultHere(req hereReq) tea.Cmd {
	mgr, root, parent := m.manager(), m.codeDir(), m.ctx
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(parent, lspWait)
		defer cancel()
		abs := filepath.Join(root, req.file)
		c, lang, err := mgr.client(ctx, abs)
		if err != nil {
			return hereMsg{req: req}
		}
		content, err := os.ReadFile(abs)
		if err != nil {
			return hereMsg{req: req}
		}
		mgr.open(c, abs, lang, string(content))
		out := hereMsg{req: req}
		switch req.kind {
		case "symbols":
			out.syms, _ = c.DocumentSymbols(ctx, abs)
		case "hover":
			lines := strings.Split(string(content), "\n")
			char := 0
			if req.line-1 < len(lines) {
				char = lsp.UTF16Column(lines[req.line-1], req.char, 4)
			}
			out.hover, _ = c.Hover(ctx, abs, req.line-1, char)
		case "callers":
			in, _, _ := c.Calls(ctx, abs, req.line, req.char)
			for _, call := range in {
				if !slices.Contains(out.callers, call.Name) {
					out.callers = append(out.callers, call.Name)
				}
			}
		}
		return out
	}
}

func (m *model) followHere() tea.Cmd {
	cur := m.current()
	if m.hereDo == nil || m.step == nil || cur.File == "" || cur.Line == 0 || cur.Kind == RowRemoved {
		return nil
	}
	var cmds []tea.Cmd
	if m.here.file != cur.File {
		m.here = hereState{file: cur.File, seq: m.here.seq}
		cmds = append(cmds, m.hereDo(hereReq{kind: "symbols", file: cur.File}))
	}
	if key := m.hereKey(); key != m.here.key {
		m.here.key = key
		m.here.seq++
		seq := m.here.seq
		cmds = append(cmds, tea.Tick(hereDelay, func(time.Time) tea.Msg { return hereTickMsg{seq} }))
	}
	return tea.Batch(cmds...)
}

func (m *model) hereKey() string {
	cur := m.current()
	word := ""
	if plain, ok := m.currentCode(); ok {
		from, to := wordBounds(plain, m.col)
		word = string([]rune(plain)[from:to])
	}
	return fmt.Sprintf("%s:%d:%s", cur.File, cur.Line, word)
}

func (m *model) hereTick(seq int) tea.Cmd {
	if seq != m.here.seq || m.hereDo == nil {
		return nil
	}
	cur := m.current()
	var cmds []tea.Cmd
	if plain, ok := m.currentCode(); ok {
		if from, to := wordBounds(plain, m.col); to > from {
			cmds = append(cmds, m.hereDo(hereReq{kind: "hover", file: cur.File, line: cur.Line, char: from}))
		}
	}
	return tea.Batch(append(cmds, m.askCallers())...)
}

func (m *model) askCallers() tea.Cmd {
	fn, ok := m.hereFunc()
	if !ok {
		m.here.callers, m.here.callersOf = nil, ""
		return nil
	}
	id := fmt.Sprintf("%s:%d", m.here.file, fn.Line)
	if id == m.here.callersOf {
		return nil
	}
	m.here.callers, m.here.callersOf = nil, id
	return m.hereDo(hereReq{kind: "callers", file: m.here.file, line: fn.Line, char: fn.Char})
}

func (m *model) handleHere(msg hereMsg) tea.Cmd {
	if msg.req.file != m.here.file {
		return nil
	}
	switch msg.req.kind {
	case "symbols":
		m.here.syms = msg.syms
		if len(msg.syms) == 0 && m.here.tries < lspRefreshes {
			m.here.tries++
			req := msg.req
			return tea.Tick(lspRefresh, func(time.Time) tea.Msg { return hereRetryMsg{req} })
		}
		return m.askCallers()
	case "hover":
		if fmt.Sprintf("%s:%d", msg.req.file, msg.req.line) == m.hereLine() {
			m.here.hover, m.here.hoverKey = msg.hover, m.here.key
		}
	case "callers":
		if fmt.Sprintf("%s:%d", msg.req.file, msg.req.line) == m.here.callersOf {
			m.here.callers = msg.callers
		}
	}
	return nil
}

type hereRetryMsg struct{ req hereReq }

func (m *model) hereLine() string {
	cur := m.current()
	return fmt.Sprintf("%s:%d", cur.File, cur.Line)
}

func (m *model) hereChain() []lsp.Symbol {
	cur := m.current()
	if cur.File != m.here.file {
		return nil
	}
	var chain []lsp.Symbol
	for _, s := range m.here.syms {
		if s.Line+1 <= cur.Line && cur.Line <= s.End+1 {
			chain = append(chain, s)
		}
	}
	slices.SortStableFunc(chain, func(a, b lsp.Symbol) int { return a.Depth - b.Depth })
	return chain
}

func (m *model) hereFunc() (lsp.Symbol, bool) {
	chain := m.hereChain()
	for i := len(chain) - 1; i >= 0; i-- {
		if callableKinds[chain[i].Kind] {
			return chain[i], true
		}
	}
	return lsp.Symbol{}, false
}

func (m *model) hereEntries(w int, row rowFunc, plain func(string) string) []sideEntry {
	chain := m.hereChain()
	hover := ""
	if m.here.hoverKey == m.here.key {
		hover = m.here.hover
	}
	if len(chain) == 0 && hover == "" {
		return nil
	}
	cur := m.current()
	where := dimStyle.Render(fmt.Sprintf("%s:%d", filepath.Base(cur.File), cur.Line))
	out := []sideEntry{{}, {text: plain(row(labelStyle.Render("HERE"), where))}}
	add := func(text string) { out = append(out, sideEntry{text: plain(row(text, ""))}) }
	for i, s := range chain {
		lead := strings.Repeat(" ", i)
		if i > 0 {
			lead = strings.Repeat(" ", i-1) + dimStyle.Render("› ")
		}
		name, kind := s.Name, s.Kind
		if i := strings.LastIndex(name, ")."); i > 0 {
			name, kind = name[i+2:], kind+" "+name[:i+1]
		}
		add(lead + boldStyle.Render(name) + faintTone.fg().Render(" · "+kind))
	}
	if n := len(m.here.callers); n > 0 {
		names := m.here.callers[:min(n, maxHereCallers)]
		text := dimStyle.Render("← ") + textTone.fg().Render(strings.Join(names, " · "))
		if n > maxHereCallers {
			text += dimStyle.Render(fmt.Sprintf(" +%d", n-maxHereCallers))
		}
		add(text)
	}
	for _, l := range hoverLines(hover) {
		for _, part := range strings.Split(ansi.Wrap(l, max(w-3, 10), ""), "\n") {
			add(codeStyle.Render(part))
		}
	}
	return out
}

func hoverLines(hover string) []string {
	var out []string
	for _, l := range strings.Split(hover, "\n") {
		l = strings.TrimSpace(ansi.Strip(l))
		if l == "" || strings.HasPrefix(l, "```") {
			continue
		}
		if out = append(out, expandTabs(l)); len(out) == maxHereHover {
			break
		}
	}
	return out
}
