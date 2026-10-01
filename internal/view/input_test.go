package view

import (
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestMultilineInput(t *testing.T) {
	m, sent := newTestModel(t)
	m.Update(key("C"))
	typeText(m, "first")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter, Alt: true})
	typeText(m, "second")
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})
	typeText(m, "third")
	if !m.composing || string(m.input) != "first\nsecond\nthird" {
		t.Fatalf("alt+enter and ctrl+j insert a new line: composing %v %q", m.composing,
			string(m.input))
	}
	for _, width := range []int{120, 150} {
		m.width = width
		lines := strings.Split(ansi.Strip(m.View()), "\n")
		i := slices.IndexFunc(lines, func(l string) bool { return strings.Contains(l, "› first") })
		if i < 0 || i+2 >= len(lines) {
			t.Fatalf("width %d: the input shows its lines:\n%s", width, strings.Join(lines, "\n"))
		}
		col := len([]rune(lines[i][:strings.Index(lines[i], "› first")])) + 2
		from := func(l string) string { return string([]rune(l)[col:]) }
		if !strings.HasPrefix(from(lines[i+1]), "second") ||
			!strings.HasPrefix(from(lines[i+2]), "third█") {
			t.Fatalf("width %d: lines line up under the text, cursor at the end:\n%s",
				width, strings.Join(lines[i:i+3], "\n"))
		}
	}
	m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.inputPos != len("first") {
		t.Fatalf("up walks the lines keeping the column: pos %d", m.inputPos)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.inputPos != len("first\nsecon") {
		t.Fatalf("down returns to the column on the next line: pos %d", m.inputPos)
	}
	m.Update(key("enter"))
	if len(*sent) != 1 || (*sent)[0].Text != "first\nsecond\nthird" {
		t.Fatalf("enter sends the text with its new lines: %+v", *sent)
	}
}

func TestInputKeepsCursorInView(t *testing.T) {
	m, _ := newTestModel(t)
	m.input = []rune("1\n2\n3\n4\n5\n6\n7\n8")
	m.inputPos = 0
	lines := m.inputLines("› ", "", 40)
	if len(lines) != inputRows || ansi.Strip(lines[0]) != "› 1" {
		t.Fatalf("a long input shows the rows around the cursor: %q", lines)
	}
	m.inputPos = len(m.input)
	lines = m.inputLines("› ", "", 40)
	if ansi.Strip(lines[len(lines)-1]) != "  8█" {
		t.Fatalf("at the end the last rows show: %q", lines)
	}
	m.input, m.inputPos = []rune(strings.Repeat("word ", 20)), 0
	for _, l := range m.inputLines("› ", "", 30) {
		if ansi.StringWidth(l) > 30 {
			t.Fatalf("a long line wraps within the width: %q", l)
		}
	}
}
