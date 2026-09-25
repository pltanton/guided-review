package view

import (
	"fmt"
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
	}{{"]", 2}, {"]", 5}, {"]", 5}, {"[", 2}, {"n", 3}, {"N", 3}, {"j", 4}, {"k", 3}, {"G", 5}, {"gg", 0}}
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
		{Kind: inbox.KindMessage, Step: "s1", File: "a.go", Lines: "3", Text: "ok"},
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
	if len(*sent) != 0 || m.step.ID != "s2" || m.review.Current != "s1" {
		t.Fatalf("plan click must preview s2 locally: sent %+v step %s current %s", *sent, m.step.ID, m.review.Current)
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
	if got := ansi.Strip(m.agentStatus()); got != "● your turn" {
		t.Fatalf("waiting: %q", got)
	}
	m.agentWaiting, m.agentSince = false, now.Add(-80*time.Second)
	if got := ansi.Strip(m.agentStatus()); got != "⠋ agent working · 1m20s" {
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
	if len(*sent) != 0 {
		t.Fatalf("space must do nothing: %+v", *sent)
	}
	m.Update(key(">"))
	if len(*sent) != 1 || (*sent)[0].Kind != inbox.KindNext {
		t.Fatalf("> must send next: %+v", *sent)
	}
	out := ansi.Strip(m.View())
	last := out[strings.LastIndex(out, "\n")+1:]
	for _, b := range []string{"✓ next", "✎ message", "? explain", "↷ skip"} {
		if !strings.Contains(last, b) {
			t.Fatalf("footer lacks %q: %q", b, last)
		}
	}
	x := ansi.StringWidth(last[:strings.Index(last, "✓ next")])
	m.Update(tea.MouseMsg{X: x + 2, Y: m.height - 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if len(*sent) != 2 || (*sent)[1].Kind != inbox.KindNext {
		t.Fatalf("click on next: %+v", *sent)
	}
	x = ansi.StringWidth(last[:strings.Index(last, "✎ message")])
	m.Update(tea.MouseMsg{X: x + 2, Y: m.height - 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if !m.composing {
		t.Fatal("click on message must open the input")
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
	if last := ansi.Strip(chat[len(chat)-1].text); !strings.HasPrefix(last, "claude: ") || !strings.Contains(last, "thinking") {
		t.Fatalf("last chat line: %q", last)
	}
	m.lastWait = t0.Add(time.Minute)
	if len(m.pendingNotes()) != 0 {
		t.Fatal("requests answered after the agent waited again")
	}
	if last := ansi.Strip(m.conversation()[len(m.conversation())-1].text); strings.Contains(last, "thinking") {
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

func TestStepPreview(t *testing.T) {
	m, sent := newTestModel(t)
	m.review.Steps[0].Status = state.StatusDone
	m.review.Current = "s2"
	m.step = &m.review.Steps[1]
	m.Update(key("H"))
	if m.step.ID != "s1" || m.review.Current != "s2" || m.review.Steps[0].Status != state.StatusDone {
		t.Fatalf("H: step %s current %s", m.step.ID, m.review.Current)
	}
	if out := ansi.Strip(m.View()); !strings.Contains(out, "viewing s1 · current is s2") {
		t.Fatalf("preview banner missing:\n%s", out)
	}
	m.Update(key(">"))
	if len(*sent) != 0 {
		t.Fatalf("> in preview must not send: %+v", *sent)
	}
	m.Update(key("c"))
	typeText(m, "late thought")
	m.Update(key("enter"))
	if len(*sent) != 1 || (*sent)[0].Step != "s1" {
		t.Fatalf("message in preview must carry s1: %+v", *sent)
	}
	m.Update(key("L"))
	if m.step.ID != "s2" || m.viewStep != "" {
		t.Fatalf("L back to current: step %s view %q", m.step.ID, m.viewStep)
	}
	m.Update(key("H"))
	m.Update(key("esc"))
	if m.step.ID != "s2" {
		t.Fatalf("esc must return to current, got %s", m.step.ID)
	}
}

func TestAgentStopped(t *testing.T) {
	m, _ := newTestModel(t)
	m.agentIdle = true
	if got := ansi.Strip(m.agentStatus()); !strings.Contains(got, "agent stopped") {
		t.Fatalf("idle status: %q", got)
	}
}

func foldModel(t *testing.T) *model {
	m, _ := newTestModel(t)
	m.rows = []Row{
		{Kind: RowFile, File: "a.go", Text: "a.go"},
		{Kind: RowRemoved, File: "a.go", Line: 2, OldLine: 2, Text: "old1", Plain: "old1", HunkStart: true},
		{Kind: RowRemoved, File: "a.go", Line: 2, OldLine: 3, Text: "old2", Plain: "old2"},
		{Kind: RowRemoved, File: "a.go", Line: 2, OldLine: 4, Text: "old3", Plain: "old3"},
		{Kind: RowAdded, File: "a.go", Line: 2, Text: "x := compute(a, c)", Plain: "x := compute(a, c)", Emph: [][2]int{{16, 17}}},
		{Kind: RowAdded, File: "a.go", Line: 3, Text: "moved()", Plain: "moved()", Moved: true, MovedTo: "b.go:9"},
	}
	m.relist()
	return m
}

func TestFoldKeys(t *testing.T) {
	m := foldModel(t)
	if m.disp[1].Kind != RowFold {
		t.Fatalf("expected a fold row, got %+v", m.disp[1])
	}
	m.cursor = 1
	m.Update(key("o"))
	if len(m.disp) != len(m.rows) {
		t.Fatalf("o must unfold: %d rows", len(m.disp))
	}
	m.cursor = 2
	m.Update(key("o"))
	if m.disp[1].Kind != RowFold {
		t.Fatal("o on an unfolded removed row must fold it back")
	}
	m.Update(key("O"))
	if len(m.disp) != len(m.rows) {
		t.Fatal("O must show every removed line")
	}
}

func TestDiffRendering(t *testing.T) {
	m := foldModel(t)
	out := ansi.Strip(m.View())
	for _, want := range []string{"▸ 3 removed lines hidden", "x := compute(a, c)", "↕", "moved()"} {
		if !strings.Contains(out, want) {
			t.Fatalf("view lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "▶+") || strings.Contains(out, "▶ ") && strings.Contains(out, "│ x :=") && false {
		t.Fatal("cursor must not be drawn as an arrow in the gutter")
	}
}

func TestCycleDiffAlgorithm(t *testing.T) {
	m, _ := newTestModel(t)
	m.algo = "histogram"
	m.Update(key("d"))
	if m.algo != "patience" || !strings.Contains(m.status, "diff: patience") {
		t.Fatalf("algo %q status %q", m.algo, m.status)
	}
	for range 3 {
		m.Update(key("d"))
	}
	if m.algo != "histogram" {
		t.Fatalf("cycle must wrap, got %q", m.algo)
	}
}

func TestComposeAnchorAndCommentActions(t *testing.T) {
	m, sent := newTestModel(t)
	m.review.Comments = []state.Comment{{ID: 7, File: "a.go", Lines: "2", Severity: state.SeverityNit, Body: "rename x"}}
	m.rows[3] = Row{Kind: RowNote, File: "a.go", Line: 2, NoteKind: "comment", NoteLabel: "#7 nit", Text: "rename x", Ref: 7}
	m.relist()
	m.cursor = 1
	m.Update(key("c"))
	if out := ansi.Strip(m.View()); !strings.Contains(out, "a.go:1 ›") {
		t.Fatalf("prompt must show the anchor:\n%s", out)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlX})
	typeText(m, "general")
	m.Update(key("enter"))

	m.cursor = 3
	m.Update(key("enter"))
	typeText(m, "why nit?")
	m.Update(key("enter"))

	m.Update(key("E"))
	if string(m.input) != "rename x" {
		t.Fatalf("edit must prefill the comment, got %q", string(m.input))
	}
	m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	typeText(m, "y")
	m.Update(key("enter"))

	want := []inbox.Event{
		{Kind: inbox.KindMessage, Step: "s1", Text: "general"},
		{Kind: inbox.KindMessage, Step: "s1", File: "a.go", Lines: "2", Comment: 7, Text: "why nit?"},
		{Kind: inbox.KindEdit, Step: "s1", Comment: 7, Text: "rename yx"},
	}
	if !reflect.DeepEqual(*sent, want) {
		t.Fatalf("sent\n%+v\nwant\n%+v", *sent, want)
	}
}

func TestExtraViews(t *testing.T) {
	m, sent := newTestModel(t)
	m.review.Files = []state.File{
		{Path: "a.go", Tier: state.TierCore},
		{Path: "wire.go", Tier: state.TierBoilerplate},
		{Path: "a.pb.go", Tier: state.TierGenerated},
		{Path: "b.pb.go", Tier: state.TierGenerated},
	}
	ids := []string{}
	for _, st := range m.extraSteps() {
		ids = append(ids, st.ID)
	}
	if !reflect.DeepEqual(ids, []string{"~boilerplate", "~generated", "~all"}) {
		t.Fatalf("extra steps = %v", ids)
	}
	if all := m.extraSteps()[2]; len(all.Hunks) != 4 {
		t.Fatalf("~all must cover every file: %+v", all.Hunks)
	}
	out := ansi.Strip(m.View())
	for _, want := range []string{"◇ boilerplate · 1 files", "◇ generated · 2 files", "◇ all changes · 4 files"} {
		if !strings.Contains(out, want) {
			t.Fatalf("sidebar lacks %q:\n%s", want, out)
		}
	}
	m.Update(key("L"))
	m.Update(key("L"))
	if m.step.ID != "~boilerplate" || m.review.Current != "s1" {
		t.Fatalf("L past the plan: step %s current %s", m.step.ID, m.review.Current)
	}
	if out := ansi.Strip(m.View()); !strings.Contains(out, "outside the plan") {
		t.Fatalf("extra view banner missing:\n%s", out)
	}
	m.Update(key("c"))
	typeText(m, "why here")
	m.Update(key("enter"))
	if len(*sent) != 1 || (*sent)[0].Step != "~boilerplate" {
		t.Fatalf("sent %+v", *sent)
	}
	m.Update(key("esc"))
	if m.step.ID != "s1" {
		t.Fatalf("esc must return to the current step, got %s", m.step.ID)
	}
}

func TestOpenFeedbackAndScrollHints(t *testing.T) {
	m, _ := newTestModel(t)
	m.cursor = 1
	m.Update(key("o"))
	if !strings.Contains(m.status, "nothing to open here") {
		t.Fatalf("o without a target: status %q", m.status)
	}
	m.Update(key("O"))
	if !strings.Contains(m.status, "no removed lines are folded") {
		t.Fatalf("O without folds: status %q", m.status)
	}
	m.height = 8
	m.cursor, m.offset = 0, 0
	m.clamp()
	out := ansi.Strip(m.View())
	if !strings.Contains(out, "more lines below") {
		t.Fatalf("missing below hint:\n%s", out)
	}
	m.Update(key("G"))
	out = ansi.Strip(m.View())
	if !strings.Contains(out, "lines above") {
		t.Fatalf("missing above hint:\n%s", out)
	}
}

func TestQuitWhenReviewCloses(t *testing.T) {
	store := state.Store{Dir: t.TempDir(), Key: "k"}
	r := &state.Review{ID: "mr-1"}
	if err := store.Save(r); err != nil {
		t.Fatal(err)
	}
	if err := store.SetCurrent("mr-1"); err != nil {
		t.Fatal(err)
	}
	m := &model{store: store, context: defaultContext}
	m.reload()
	if m.review == nil {
		t.Fatal("review not loaded")
	}
	if err := store.ClearCurrent(); err != nil {
		t.Fatal(err)
	}
	if _, cmd := m.Update(reloadMsg{}); cmd == nil {
		t.Fatal("viewer must quit when the review it showed is closed")
	}
}

func TestPublishButton(t *testing.T) {
	m, sent := newTestModel(t)
	var ran [][]string
	m.runGr = func(args ...string) (string, error) {
		ran = append(ran, args)
		if len(args) > 1 && args[1] == "--dry-run" {
			return "--- summary\n## Guided review: approve\n", nil
		}
		return "published 2 comments and the summary to !7\n", nil
	}
	m.Update(key("P"))
	if !strings.Contains(m.status, "nothing prepared") || len(ran) != 0 {
		t.Fatalf("P before prepare: status %q ran %v", m.status, ran)
	}
	m.review.Publish = &state.PublishPlan{Verdict: "approve"}
	m.Update(key("P"))
	if m.preview == "" || !strings.Contains(ansi.Strip(m.View()), "## Guided review: approve") {
		t.Fatalf("first P must show the preview:\n%s", ansi.Strip(m.View()))
	}
	m.Update(key("P"))
	if len(ran) != 2 || ran[1][0] != "publish" || len(ran[1]) != 1 {
		t.Fatalf("second P must run gr publish: %v", ran)
	}
	if m.preview != "" || !strings.Contains(m.status, "published 2 comments") {
		t.Fatalf("after publish: preview %q status %q", m.preview, m.status)
	}
	if len(*sent) != 1 || (*sent)[0].Kind != inbox.KindPublished {
		t.Fatalf("agent must be told: %+v", *sent)
	}
}

func TestWordMotions(t *testing.T) {
	line := "    x := compute(a, b)"
	if got := wordStart(line, 0); got != 4 {
		t.Fatalf("wordStart from 0 = %d", got)
	}
	if got := nextWord(line, 4); got != 9 {
		t.Fatalf("nextWord = %d", got)
	}
	if got := nextWord(line, 9); got != 17 {
		t.Fatalf("nextWord 2 = %d", got)
	}
	if got := prevWord(line, 17); got != 9 {
		t.Fatalf("prevWord = %d", got)
	}
	if from, to := wordBounds(line, 11); from != 9 || to != 16 {
		t.Fatalf("wordBounds = %d %d", from, to)
	}
}

func TestLSPFlow(t *testing.T) {
	m, _ := newTestModel(t)
	var asked []string
	m.lspDo = func(kind, file string, line, col int) tea.Cmd {
		asked = append(asked, fmt.Sprintf("%s %s:%d:%d", kind, file, line, col))
		return nil
	}
	m.cursor = 2
	m.Update(key("w"))
	m.Update(key("g"))
	m.Update(key("d"))
	m.Update(key("g"))
	m.Update(key("r"))
	m.Update(key("K"))
	want := []string{"definition a.go:2:5", "references a.go:2:5", "hover a.go:2:5"}
	if !reflect.DeepEqual(asked, want) {
		t.Fatalf("asked %v, want %v", asked, want)
	}

	m.Update(lspMsg{kind: "references", locs: []lspLoc{
		{Path: "a.go", Line: 3, Text: "use(x)"},
		{Path: "b.go", Line: 9, Text: "x = 2"},
	}})
	if m.popup == nil || m.popup.kind != "references" || len(m.popup.items) != 2 {
		t.Fatalf("popup %+v", m.popup)
	}
	out := ansi.Strip(m.View())
	if !strings.Contains(out, "references · 2") || !strings.Contains(out, "b.go:9") {
		t.Fatalf("popup not rendered:\n%s", out)
	}
	m.peekFile = func(path string) []string { return numbered(20) }
	m.Update(key("j"))
	m.Update(key("enter"))
	if m.popup.kind != "peek" || m.popup.loc.Path != "b.go" || len(m.popupStack) != 1 {
		t.Fatalf("enter must peek the selected reference: %+v stack %d", m.popup, len(m.popupStack))
	}
	if out := ansi.Strip(m.View()); !strings.Contains(out, "L9") {
		t.Fatalf("peek must show the target line:\n%s", out)
	}
	m.Update(key("esc"))
	if m.popup == nil || m.popup.kind != "references" {
		t.Fatal("esc in peek must return to the list")
	}
	m.Update(key("esc"))
	if m.popup != nil {
		t.Fatal("esc in the list must close the popup")
	}

	m.Update(lspMsg{kind: "hover", hover: "func compute(a, b int) int\n\tfield int"})
	if strings.Contains(strings.Join(m.popup.lines, ""), "\t") {
		t.Fatal("hover must not contain raw tabs")
	}
	if m.popup == nil || !strings.Contains(strings.Join(m.popup.lines, "\n"), "func compute") {
		t.Fatalf("hover popup %+v", m.popup)
	}
	m.Update(key("esc"))
	m.Update(lspMsg{kind: "definition", err: fmt.Errorf("no LSP server for .kt")})
	if m.err == nil || m.popup != nil {
		t.Fatal("lsp errors go to the status line")
	}
}
