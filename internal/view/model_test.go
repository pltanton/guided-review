package view

import (
	"reflect"
	"strings"
	"testing"
	"time"

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

func TestIntakeBeforePlan(t *testing.T) {
	var sent []inbox.Event
	r := &state.Review{ID: "mr-1", MR: &state.MR{IID: 1, Title: "Add guard"},
		Messages: []state.Message{{Text: "Task: reject negatives. Верно понял?"}}}
	m := &model{review: r, width: 100, height: 20, showPlan: true,
		send: func(e inbox.Event) error { sent = append(sent, e); return nil }}
	out := ansi.Strip(m.View())
	for _, want := range []string{"review mr-1", "Add guard", "claude: Task: reject negatives. Верно понял?"} {
		if !strings.Contains(out, want) {
			t.Fatalf("intake view lacks %q:\n%s", want, out)
		}
	}
	m.Update(key("c"))
	typeText(m, "да")
	m.Update(key("enter"))
	if len(sent) != 1 || sent[0] != (inbox.Event{Kind: inbox.KindMessage, Text: "да"}) {
		t.Fatalf("sent %+v", sent)
	}
	m.Update(key(">"))
	if len(sent) != 1 {
		t.Fatalf("next without a plan must not be sent: %+v", sent)
	}
}

func TestAgentStatus(t *testing.T) {
	m, _ := newTestModel(t)
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	m.now = func() time.Time { return now }
	m.agentWaiting, m.agentSince = true, now.Add(-10*time.Second)
	if got := ansi.Strip(m.agentStatus()); got != "● ждёт тебя" {
		t.Fatalf("waiting: %q", got)
	}
	m.agentWaiting, m.agentSince = false, now.Add(-80*time.Second)
	if got := ansi.Strip(m.agentStatus()); got != "⠋ агент работает · 1m20s" {
		t.Fatalf("working: %q", got)
	}
	m.review.Progress = &state.Progress{Text: "строю план: читаю diff", Time: now.Add(-5 * time.Second)}
	m.frame = 2
	if got := ansi.Strip(m.agentStatus()); got != "⠹ строю план: читаю diff · 1m20s" {
		t.Fatalf("progress: %q", got)
	}
	m.review.Steps, m.step = nil, nil
	if out := ansi.Strip(m.View()); !strings.Contains(out, "⠹ строю план: читаю diff · 1m20s") {
		t.Fatalf("intake view lacks spinner:\n%s", out)
	}
}

func TestNotesWrapIntoBlocks(t *testing.T) {
	m, _ := newTestModel(t)
	m.width, m.showPlan = 60, false
	m.rows[3] = Row{Kind: RowNote, File: "a.go", Line: 2, NoteKind: "comment", NoteLabel: "minor",
		Text: "Event with the same source verdict and a different command becomes a separate key and looks identical in logs"}
	m.relist()
	var heads, conts int
	for _, r := range m.disp {
		if r.Kind != RowNote {
			continue
		}
		if r.NoteHead {
			heads++
		} else {
			conts++
		}
	}
	if heads != 1 || conts < 1 {
		t.Fatalf("heads %d conts %d: %+v", heads, conts, m.disp)
	}
	out := ansi.Strip(m.View())
	if !strings.Contains(out, "▌  minor  Event with the same") || strings.Contains(out, "…") {
		t.Fatalf("note block:\n%s", out)
	}
	for _, line := range strings.Split(out, "\n") {
		if ansi.StringWidth(line) > m.width {
			t.Fatalf("line wider than %d: %q", m.width, line)
		}
	}
	m.cursor = 0
	m.Update(key("n"))
	m.Update(key("n"))
	if !m.disp[m.cursor].NoteHead {
		t.Fatal("n must land on note heads only")
	}
}

func TestButtons(t *testing.T) {
	m, sent := newTestModel(t)
	m.Update(key(" "))
	if len(*sent) != 1 || (*sent)[0].Kind != inbox.KindNext {
		t.Fatalf("space must send next: %+v", *sent)
	}
	out := ansi.Strip(m.View())
	last := out[strings.LastIndex(out, "\n")+1:]
	for _, b := range []string{"✓ дальше", "✎ написать", "? поясни", "↷ пропустить"} {
		if !strings.Contains(last, b) {
			t.Fatalf("footer lacks %q: %q", b, last)
		}
	}
	x := ansi.StringWidth(last[:strings.Index(last, "✓ дальше")])
	m.Update(tea.MouseMsg{X: x + 2, Y: m.height - 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if len(*sent) != 2 || (*sent)[1].Kind != inbox.KindNext {
		t.Fatalf("click on дальше: %+v", *sent)
	}
	x = ansi.StringWidth(last[:strings.Index(last, "✎ написать")])
	m.Update(tea.MouseMsg{X: x + 2, Y: m.height - 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if !m.composing {
		t.Fatal("click on написать must open the input")
	}
}

func TestPendingRequests(t *testing.T) {
	m, _ := newTestModel(t)
	t0 := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	m.src = nil
	m.lastWait = t0
	m.events = []inbox.Event{
		{Time: t0.Add(-time.Minute), Kind: inbox.KindExplain, Step: "s1", File: "a.go", Lines: "1"},
		{Time: t0.Add(time.Second), Kind: inbox.KindExplain, Step: "s1", File: "a.go", Lines: "3"},
		{Time: t0.Add(2 * time.Second), Kind: inbox.KindMessage, Step: "s1", Text: "why?"},
	}
	notes := m.pendingNotes()
	if len(notes) != 1 || notes[0].Line != 3 || notes[0].Kind != "pending" {
		t.Fatalf("pending notes: %+v", notes)
	}
	chat := m.conversation()
	if last := ansi.Strip(chat[len(chat)-1].text); !strings.HasPrefix(last, "claude: ") || !strings.Contains(last, "думает") {
		t.Fatalf("last chat line: %q", last)
	}
	m.lastWait = t0.Add(time.Minute)
	if len(m.pendingNotes()) != 0 {
		t.Fatal("requests answered after the agent waited again")
	}
	if last := ansi.Strip(m.conversation()[len(m.conversation())-1].text); strings.Contains(last, "думает") {
		t.Fatalf("stale thinking line: %q", last)
	}
}

func twoFileModel(t *testing.T) (*model, *[]inbox.Event) {
	m, sent := newTestModel(t)
	m.rows = append(testRows(),
		Row{Kind: RowFile, File: "api/b.go", Text: "api/b.go"},
		Row{Kind: RowAdded, File: "api/b.go", Line: 4, Text: "b := 4", HunkStart: true},
		Row{Kind: RowFile, File: "api/c.go", Text: "api/c.go"},
		Row{Kind: RowAdded, File: "api/c.go", Line: 7, Text: "c := 7", HunkStart: true},
	)
	m.relist()
	return m, sent
}

func TestFilesPanel(t *testing.T) {
	m, _ := twoFileModel(t)
	if got := m.stepFiles(); !reflect.DeepEqual(got, []string{"a.go", "api/b.go", "api/c.go"}) {
		t.Fatalf("stepFiles = %v", got)
	}
	m.Update(key("}"))
	if it := m.current(); it.File != "api/b.go" {
		t.Fatalf("} → %+v", it)
	}
	m.Update(key("}"))
	m.Update(key("{"))
	if it := m.current(); it.File != "api/b.go" {
		t.Fatalf("{ → %+v", it)
	}
	m.Update(key("f"))
	if !m.focusFiles {
		t.Fatal("f must focus the files panel")
	}
	m.Update(key("j"))
	m.Update(key("enter"))
	if m.focusFiles || m.current().File != "api/c.go" {
		t.Fatalf("enter in files panel: focus %v item %+v", m.focusFiles, m.current())
	}
	out := ansi.Strip(m.View())
	for _, want := range []string{"files", "api/", "b.go", "c.go"} {
		if !strings.Contains(out, want) {
			t.Fatalf("view lacks %q:\n%s", want, out)
		}
	}
	y := strings.Split(out, "\n")
	for i, line := range y {
		if strings.Contains(line, "  a.go") || strings.HasPrefix(strings.TrimSpace(line), "a.go") {
			m.Update(tea.MouseMsg{X: 3, Y: i, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
			break
		}
	}
	if m.current().File != "a.go" {
		t.Fatalf("click on a.go → %+v", m.current())
	}
}

func TestResolvedDiscussionsHidden(t *testing.T) {
	m, _ := newTestModel(t)
	m.review.Discussions = []state.Discussion{
		{Author: "a", Body: "open one", File: "a.go", Line: 1},
		{Author: "b", Body: "done one", File: "a.go", Line: 1, Resolved: true},
	}
	var texts []string
	for _, n := range m.notes() {
		texts = append(texts, n.Text)
	}
	if !reflect.DeepEqual(texts, []string{"open one"}) {
		t.Fatalf("notes = %v", texts)
	}
}
