package view

import (
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/pltanton/guided-review/internal/inbox"
	"github.com/pltanton/guided-review/internal/state"
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
		i := slices.IndexFunc(lines, func(l string) bool { return strings.Contains(l, "▌ first") })
		if i < 0 || i+2 >= len(lines) {
			t.Fatalf("width %d: the input shows its lines:\n%s", width, strings.Join(lines, "\n"))
		}
		col := len([]rune(lines[i][:strings.Index(lines[i], "▌ first")])) + 2
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

func TestComposeStatusline(t *testing.T) {
	cases := []struct {
		name   string
		set    func(m *model)
		narrow string
		wide   string
	}{
		{"message", func(m *model) {
			m.composeKind, m.anchorFile, m.anchorLines = inbox.KindMessage, "internal/a.go", "2-4"
		}, " COMMENT a.go:2-4   enter send · tab ask", "alt+enter new line · esc cancel"},
		{"reply to a comment", func(m *model) {
			m.composeKind, m.composeRef, m.anchorFile, m.anchorLines = inbox.KindMessage, 7, "a.go", "2"
		}, " COMMENT re #7 a.go:2         enter send",
			"ctrl+r raw · alt+enter new line · esc cancel"},
		{"general", func(m *model) { m.composeKind = inbox.KindMessage },
			" MSG             enter send · ctrl+r raw", "alt+enter new line · esc cancel"},
		{"paused", func(m *model) { m.composeKind, m.interrupted = inbox.KindMessage, true },
			" MSG paused      enter send · ctrl+r raw", "esc cancel"},
		{"ask", func(m *model) {
			m.composeKind, m.anchorFile, m.anchorLines = inbox.KindAsk, "a.go", "2"
		}, " ASK a.go:2                   enter send",
			"enter alone explains · tab comment · alt+enter new line · esc cancel"},
		{"raw", func(m *model) {
			m.composeKind, m.raw, m.anchorFile, m.anchorLines = inbox.KindMessage, true, "a.go", "2"
		}, " RAW minor a.go:2             enter save",
			"tab severity · ctrl+r via agent · alt+enter new line · esc cancel"},
		{"edit", func(m *model) { m.composeKind, m.composeRef = inbox.KindEdit, 3 },
			" EDIT #3         enter save · ctrl+r raw", "alt+enter new line · esc cancel"},
		{"skip", func(m *model) { m.composeKind = inbox.KindSkip },
			" SKIP    enter skip · alt+enter new line", "enter skip · alt+enter new line · esc cancel"},
		{"thread reply", func(m *model) {
			m.review.Discussions = []state.Discussion{{ID: "d1", File: "x/b.go", Line: 9}}
			m.composeKind, m.composeThread = kindThreadReply, "d1"
		}, " REPLY b.go:9 ",
			"enter reply, thread stays open · alt+enter new line · esc cancel"},
		{"thread question", func(m *model) {
			m.review.Discussions = []state.Discussion{{ID: "d1", Author: "bob"}}
			m.composeKind, m.composeThread = inbox.KindMessage, "d1"
		}, " THREAD @bob                  enter send", "enter send · alt+enter new line · esc cancel"},
	}
	for _, c := range cases {
		m, _ := newTestModel(t)
		m.composing = true
		c.set(m)
		narrow := ansi.Strip(m.composeStatus(40))
		if narrow != c.narrow {
			t.Errorf("%s at 40: %q, want %q", c.name, narrow, c.narrow)
		}
		if wide := ansi.Strip(m.composeStatus(120)); !strings.HasSuffix(wide, c.wide) ||
			ansi.StringWidth(wide) != 120 {
			t.Errorf("%s at 120: %q, want it to end in %q", c.name, wide, c.wide)
		}
		if tiny := ansi.Strip(m.composeStatus(8)); ansi.StringWidth(tiny) > 8 ||
			!strings.HasPrefix(tiny, " "+c.narrow[1:3]) {
			t.Errorf("%s at 8 keeps the badge: %q", c.name, tiny)
		}
	}
}

func TestComposeStatuslinePlacement(t *testing.T) {
	m, _ := newTestModel(t)
	m.cursor = 2
	m.Update(key("C"))
	typeText(m, "hello there")
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	if n := len(lines); n != m.height || strings.TrimSpace(lines[n-2]) != "▌ hello there█" ||
		!strings.HasPrefix(lines[n-1], " MSG ") ||
		!strings.HasSuffix(lines[n-1], "esc cancel") {
		t.Fatalf("bottom: the text has its line, the statusline sits below:\n%s",
			strings.Join(lines, "\n"))
	}

	m.width = 150
	lines = strings.Split(ansi.Strip(m.View()), "\n")
	side := func(l string) string {
		return strings.TrimSpace(string([]rune(l)[m.width-m.chatWidth()+1:]))
	}
	n := len(lines)
	if n != m.height || side(lines[n-3]) != "▌ hello there█" ||
		!strings.HasPrefix(side(lines[n-2]), "MSG ") {
		t.Fatalf("side chat: same input and statusline:\n%s", strings.Join(lines, "\n"))
	}

	for _, size := range [][2]int{{150, 3}, {150, 6}, {120, 4}, {60, 2}, {30, 8}} {
		m.width, m.height = size[0], size[1]
		m.input = []rune(strings.Repeat("long words ", 40))
		if got := len(strings.Split(m.View(), "\n")); got > max(m.height, 1)+inputRows+1 {
			t.Fatalf("%dx%d: %d rows", size[0], size[1], got)
		}
		if h := m.bodyHeight(); h < 1 {
			t.Fatalf("%dx%d: body height %d", size[0], size[1], h)
		}
		if got := m.sideChatLines(m.height, 40); len(got) != m.height {
			t.Fatalf("%dx%d: side chat has %d rows", size[0], size[1], len(got))
		}
	}
}

func TestShiftEnterAddsLine(t *testing.T) {
	seqs := []string{"\x1b[27;2;13~", "\x1b[13;2u", "\x1b[27;5;13~"}
	msgs := parseInput(t, strings.Join(seqs, "")+"\r", len(seqs)+1)
	if len(msgs) != len(seqs)+1 {
		t.Fatalf("bubbletea parsed %d messages: %v", len(msgs), msgs)
	}
	m, sent := newTestModel(t)
	m.Update(key("C"))
	typeText(m, "a")
	for _, msg := range msgs[:len(seqs)] {
		m.Update(msg)
		typeText(m, "b")
	}
	if !m.composing || string(m.input) != "a\nb\nb\nb" {
		t.Fatalf("shift+enter adds a line: composing %v %q", m.composing, string(m.input))
	}
	m.Update(msgs[len(seqs)])
	if m.composing || len(*sent) != 1 || (*sent)[0].Text != "a\nb\nb\nb" {
		t.Fatalf("plain enter still sends: %+v", *sent)
	}
	if _, ok := modifiedEnter("?CSI[50 55 59 53 59 57 126]?"); ok {
		t.Fatal("ctrl+tab is not enter")
	}
	if mod, ok := modifiedEnter("?CSI[49 51 117]?"); !ok || mod != 0 {
		t.Fatalf("an unmodified CSI u enter is enter: %d %v", mod, ok)
	}
}

type inputProbe struct {
	want int
	got  *[]tea.Msg
}

func (p inputProbe) Init() tea.Cmd { return nil }

func (p inputProbe) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(fmt.Stringer); ok {
		*p.got = append(*p.got, msg)
	}
	if len(*p.got) == p.want {
		return p, tea.Quit
	}
	return p, nil
}

func (p inputProbe) View() string { return "" }

func parseInput(t *testing.T, in string, n int) []tea.Msg {
	t.Helper()
	var got []tea.Msg
	r, w := io.Pipe()
	p := tea.NewProgram(inputProbe{n, &got}, tea.WithInput(r), tea.WithOutput(io.Discard),
		tea.WithoutRenderer(), tea.WithoutSignals())
	go func() { _, _ = w.Write([]byte(in)) }()
	timer := time.AfterFunc(2*time.Second, p.Quit)
	defer timer.Stop()
	if _, err := p.Run(); err != nil {
		t.Fatal(err)
	}
	return got
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

func TestInlineComposer(t *testing.T) {
	m, sent := newTestModel(t)
	m.cursor = 2
	m.Update(key("enter"))
	typeText(m, "why x")
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	at := slices.IndexFunc(lines, func(l string) bool { return strings.Contains(l, "x := 1") })
	if at < 0 || at+2 >= len(lines) || !strings.Contains(lines[at+1], "▌ why x█") ||
		!strings.Contains(lines[at+2], "COMMENT a.go:2") || !strings.Contains(lines[at+2], "tab ask") {
		t.Fatalf("the composer sits under its line:\n%s", strings.Join(lines, "\n"))
	}
	if strings.Contains(lines[len(lines)-1], "COMMENT") {
		t.Fatal("the bottom prompt stays the footer while composing inline")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.composeKind != inbox.KindAsk ||
		!strings.Contains(ansi.Strip(m.View()), "ASK a.go:2") {
		t.Fatal("tab switches to ask")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.composeKind != inbox.KindMessage {
		t.Fatal("tab switches back to a comment")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m.Update(key("enter"))
	want := inbox.Event{Kind: inbox.KindAsk, Step: "s1", File: "a.go", Lines: "2", Text: "why x"}
	if len(*sent) != 1 || (*sent)[0] != want {
		t.Fatalf("sent %+v, want %+v", *sent, want)
	}
}
