package view

import (
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/aplotnikov/guided-review/internal/inbox"
	"github.com/aplotnikov/guided-review/internal/state"
)

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	case " ":
		return tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func typeText(m *model, s string) {
	for _, r := range s {
		m.Update(key(string(r)))
	}
}

func testRows() []Row {
	return []Row{
		{Kind: RowFile, File: "a.go", Text: "a.go"},
		{Kind: RowCode, File: "a.go", Line: 1, OldLine: 1, Text: "package a"},
		{Kind: RowAdded, File: "a.go", Line: 2, Text: "x := 1", HunkStart: true},
		{Kind: RowNote, File: "a.go", Line: 2, Text: "why x", NoteKind: "note"},
		{Kind: RowCode, File: "a.go", Line: 3, OldLine: 2, Text: "y := 2"},
		{Kind: RowAdded, File: "a.go", Line: 9, Text: "z := 3", HunkStart: true},
	}
}

func newTestModel(t *testing.T) (*model, *[]inbox.Event) {
	t.Helper()
	var sent []inbox.Event
	r := &state.Review{ID: "mr-1", Current: "s1", Steps: []state.Step{
		{ID: "s1", Title: "first", Kind: "logic", Status: state.StatusPending},
		{ID: "s2", Title: "second", Kind: "logic", Status: state.StatusPending},
	}}
	m := &model{
		review: r, step: &r.Steps[0], rows: testRows(), width: 120, height: 30,
		context: defaultContext, showPlan: true,
		send: func(e inbox.Event) error { sent = append(sent, e); return nil },
	}
	m.split = pairRows(m.rows)
	m.relist()
	return m, &sent
}

func TestNavigation(t *testing.T) {
	m, _ := newTestModel(t)
	steps := []struct {
		key  string
		want int
	}{{"]", 2}, {"]", 5}, {"]", 5}, {"[", 2}, {"n", 3}, {"N", 3}, {"j", 4}, {"k", 3}, {"G", 5}, {"g", 0}}
	for _, s := range steps {
		m.Update(key(s.key))
		if m.cursor != s.want {
			t.Fatalf("after %q cursor = %d, want %d", s.key, m.cursor, s.want)
		}
	}
	if _, cmd := m.Update(key("q")); cmd == nil {
		t.Fatal("q must quit")
	}
}

func TestEventsFromKeys(t *testing.T) {
	m, sent := newTestModel(t)
	m.cursor = 2
	m.Update(key("?"))
	m.Update(key("v"))
	m.Update(key("j"))
	m.Update(key("j"))
	m.Update(key("c"))
	typeText(m, "is this safe")
	m.Update(key("backspace"))
	m.Update(key("e"))
	m.Update(key("enter"))
	m.Update(key("c"))
	typeText(m, "ok")
	m.Update(key("enter"))
	m.Update(key("S"))
	typeText(m, "trivial")
	m.Update(key("enter"))
	m.Update(key(">"))
	m.Update(key("c"))
	typeText(m, "dropped")
	m.Update(key("esc"))

	want := []inbox.Event{
		{Kind: inbox.KindExplain, Step: "s1", File: "a.go", Lines: "2"},
		{Kind: inbox.KindMessage, Step: "s1", File: "a.go", Lines: "2-3", Text: "is this safe"},
		{Kind: inbox.KindMessage, Step: "s1", Text: "ok"},
		{Kind: inbox.KindSkip, Step: "s1", Text: "trivial"},
		{Kind: inbox.KindNext, Step: "s1"},
	}
	if !reflect.DeepEqual(*sent, want) {
		t.Fatalf("sent\n%+v\nwant\n%+v", *sent, want)
	}
	if m.composing || m.visual {
		t.Fatal("compose and visual must be off after sending")
	}
}

func TestComposeKeysDoNotNavigate(t *testing.T) {
	m, _ := newTestModel(t)
	m.Update(key("c"))
	typeText(m, "jq ]")
	if m.cursor != 0 || string(m.input) != "jq ]" {
		t.Fatalf("cursor %d input %q", m.cursor, string(m.input))
	}
}

func TestMouse(t *testing.T) {
	m, sent := newTestModel(t)
	hdr := len(m.header())
	pw := m.planWidth()
	m.Update(tea.MouseMsg{X: pw + 5, Y: hdr + 4, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.cursor != 4 {
		t.Fatalf("click: cursor = %d, want 4", m.cursor)
	}
	m.Update(tea.MouseMsg{X: pw + 5, Y: hdr + 5, Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion})
	m.Update(tea.MouseMsg{X: pw + 5, Y: hdr + 5, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease})
	if file, lines, ok := m.selection(); !ok || file != "a.go" || lines != "3-9" {
		t.Fatalf("drag selection = %q %q %v", file, lines, ok)
	}
	m.Update(tea.MouseMsg{X: 1, Y: 2, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if len(*sent) != 1 || (*sent)[0] != (inbox.Event{Kind: inbox.KindGoto, Step: "s2"}) {
		t.Fatalf("plan click sent %+v", *sent)
	}
}

func TestViewRenders(t *testing.T) {
	m, _ := newTestModel(t)
	m.review.Messages = []state.Message{{Step: "s1", Text: "Adds x and y."}}
	out := ansi.Strip(m.View())
	for _, want := range []string{"▶ s1 first", "· s2 second", "s1 1/2 logic · first", "why x", "claude: Adds x and y."} {
		if !strings.Contains(out, want) {
			t.Fatalf("view lacks %q:\n%s", want, out)
		}
	}
	if lines := strings.Count(out, "\n") + 1; lines != m.height {
		t.Fatalf("view has %d lines, want %d", lines, m.height)
	}
	m.Update(key("s"))
	out = ansi.Strip(m.View())
	if !strings.Contains(out, "package a") || !m.useSplit() {
		t.Fatalf("split view:\n%s", out)
	}
}

func TestFirstFocus(t *testing.T) {
	tests := []struct {
		name  string
		items []item
		want  int
	}{
		{"hunk start", []item{{File: "a.go"}, {File: "a.go", Line: 1}, {File: "a.go", Line: 2, HunkStart: true}}, 2},
		{"hunk outside window", []item{{File: "a.go"}, {File: "a.go", Line: 76}}, 1},
		{"empty", nil, 0},
	}
	for _, tt := range tests {
		if got := firstFocus(tt.items); got != tt.want {
			t.Errorf("%s: firstFocus = %d, want %d", tt.name, got, tt.want)
		}
	}
}

func TestGroupedRunes(t *testing.T) {
	m, sent := newTestModel(t)
	m.Update(key("jjj"))
	if m.cursor != 3 {
		t.Fatalf("grouped jjj: cursor = %d, want 3", m.cursor)
	}
	m.Update(key("gchi"))
	m.Update(key("enter"))
	if len(*sent) != 1 || (*sent)[0].Text != "hi" {
		t.Fatalf("grouped compose sent %+v", *sent)
	}
}
