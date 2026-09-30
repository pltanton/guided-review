package view

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/pltanton/guided-review/internal/config"
	"github.com/pltanton/guided-review/internal/inbox"
	"github.com/pltanton/guided-review/internal/lsp"
	"github.com/pltanton/guided-review/internal/state"
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
	m.Update(key("enter"))
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
	m.Update(
		tea.MouseMsg{
			X:      pw + 5,
			Y:      hdr + 4,
			Button: tea.MouseButtonLeft,
			Action: tea.MouseActionPress,
		},
	)
	if m.cursor != 4 {
		t.Fatalf("click: cursor = %d, want 4", m.cursor)
	}
	m.Update(
		tea.MouseMsg{
			X:      pw + 5,
			Y:      hdr + 5,
			Button: tea.MouseButtonLeft,
			Action: tea.MouseActionMotion,
		},
	)
	m.Update(
		tea.MouseMsg{
			X:      pw + 5,
			Y:      hdr + 5,
			Button: tea.MouseButtonLeft,
			Action: tea.MouseActionRelease,
		},
	)
	if file, lines, ok := m.selection(); !ok || file != "a.go" || lines != "3-9" {
		t.Fatalf("drag selection = %q %q %v", file, lines, ok)
	}
	m.Update(tea.MouseMsg{X: 1, Y: 2, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if len(*sent) != 0 || m.step.ID != "s2" || m.review.Current != "s1" {
		t.Fatalf(
			"plan click must preview s2 locally: sent %+v step %s current %s",
			*sent,
			m.step.ID,
			m.review.Current,
		)
	}
}

func TestViewRenders(t *testing.T) {
	m, _ := newTestModel(t)
	m.review.Messages = []state.Message{{Step: "s1", Text: "Adds x and y."}}
	out := ansi.Strip(m.View())
	for _, want := range []string{
		"▶ s1 first", "○ s2 second", " s1  first  logic", "1/2", "why x", "claude │ Adds x and y.",
	} {
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
		items []line
		want  int
	}{
		{
			"hunk start",
			unifiedLines(
				[]Row{
					{File: "a.go"},
					{File: "a.go", Line: 1},
					{File: "a.go", Line: 2, HunkStart: true},
				},
			),
			2,
		},
		{"hunk outside window", unifiedLines([]Row{{File: "a.go"}, {File: "a.go", Line: 76}}), 1},
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
	m.Update(key("jchi"))
	m.Update(key("enter"))
	if len(*sent) != 1 || (*sent)[0].Text != "hi" {
		t.Fatalf("grouped compose sent %+v", *sent)
	}
}

func TestIntakeBeforePlan(t *testing.T) {
	var sent []inbox.Event
	r := &state.Review{ID: "mr-1", MR: &state.MR{IID: 1, Title: "Add guard"},
		Messages: []state.Message{{Text: "Task: reject negatives. Верно понял?"}},
		Files: []state.File{
			{Path: "a.go", Added: 10, Deleted: 2},
			{Path: "a.pb.go", Tier: state.TierGenerated, Added: 300},
		},
		Discussions: []state.Discussion{{Author: "bob"}, {Author: "ci", Resolved: true}}}
	m := &model{review: r, width: 100, height: 20, showPlan: true,
		send: func(e inbox.Event) error { sent = append(sent, e); return nil }}
	out := ansi.Strip(m.View())
	for _, want := range []string{
		"guided review · MR !1 Add guard", "● task & plan  ›  ○ steps  ›  ○ finish",
		"2 files  +310 −2   core 1 · boilerplate 0 · generated 1", "1 open discussions",
		"claude │ Task: reject negatives. Верно понял?",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("intake view lacks %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "› press c or enter to answer the agent") {
		t.Fatalf("intake must show where to answer:\n%s", out)
	}
	if strings.Contains(out, "── intake ──") || len(strings.Split(out, "\n")) != 20 {
		t.Fatalf("intake must fill the screen without a chat section label:\n%s", out)
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
	m.review.Progress = &state.Progress{
		Text: "строю план: читаю diff",
		Time: now.Add(-5 * time.Second),
	}
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
	for _, r := range m.lines {
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
		t.Fatalf("heads %d conts %d: %+v", heads, conts, m.lines)
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
	if !m.lines[m.cursor].NoteHead {
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
	for _, b := range []string{"✓ next", "✎ message", "? ask", "↷ skip"} {
		if !strings.Contains(last, b) {
			t.Fatalf("footer lacks %q: %q", b, last)
		}
	}
	x := ansi.StringWidth(last[:strings.Index(last, "✓ next")])
	m.Update(
		tea.MouseMsg{
			X:      x + 2,
			Y:      m.height - 1,
			Button: tea.MouseButtonLeft,
			Action: tea.MouseActionPress,
		},
	)
	if len(*sent) != 2 || (*sent)[1].Kind != inbox.KindNext {
		t.Fatalf("click on next: %+v", *sent)
	}
	x = ansi.StringWidth(last[:strings.Index(last, "✎ message")])
	m.Update(
		tea.MouseMsg{
			X:      x + 2,
			Y:      m.height - 1,
			Button: tea.MouseButtonLeft,
			Action: tea.MouseActionPress,
		},
	)
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
	pendingNotes := func() []Note {
		var out []Note
		for _, n := range m.notes() {
			if n.Kind == "pending" {
				out = append(out, n)
			}
		}
		return out
	}
	notes := pendingNotes()
	if len(notes) != 1 || notes[0].Line != 3 || notes[0].Kind != "pending" {
		t.Fatalf("pending notes: %+v", notes)
	}
	chat := m.conversation(false)
	if last := chat[len(chat)-1]; last.you || !strings.Contains(last.text, "thinking") {
		t.Fatalf("last chat line: %+v", last)
	}
	m.lastWait = t0.Add(time.Minute)
	if len(pendingNotes()) != 0 {
		t.Fatal("requests answered after the agent waited again")
	}
	chat = m.conversation(false)
	if last := ansi.Strip(chat[len(chat)-1].text); strings.Contains(last, "thinking") {
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
	for _, want := range []string{"FILES", "api/", "b.go", "c.go"} {
		if !strings.Contains(out, want) {
			t.Fatalf("view lacks %q:\n%s", want, out)
		}
	}
	y := strings.Split(out, "\n")
	for i, line := range y {
		if strings.Contains(line, "  a.go") || strings.HasPrefix(strings.TrimSpace(line), "a.go") {
			m.Update(
				tea.MouseMsg{
					X:      3,
					Y:      i,
					Button: tea.MouseButtonLeft,
					Action: tea.MouseActionPress,
				},
			)
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
	if m.step.ID != "s1" || m.review.Current != "s2" ||
		m.review.Steps[0].Status != state.StatusDone {
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
		{
			Kind:      RowRemoved,
			File:      "a.go",
			Line:      2,
			OldLine:   2,
			Text:      "old1",
			Plain:     "old1",
			HunkStart: true,
		},
		{Kind: RowRemoved, File: "a.go", Line: 2, OldLine: 3, Text: "old2", Plain: "old2"},
		{Kind: RowRemoved, File: "a.go", Line: 2, OldLine: 4, Text: "old3", Plain: "old3"},
		{
			Kind:  RowAdded,
			File:  "a.go",
			Line:  2,
			Text:  "x := compute(a, c)",
			Plain: "x := compute(a, c)",
			Emph:  [][2]int{{16, 17}},
		},
		{
			Kind:    RowAdded,
			File:    "a.go",
			Line:    3,
			Text:    "moved()",
			Plain:   "moved()",
			Moved:   true,
			MovedTo: "b.go:9",
		},
	}
	m.relist()
	return m
}

func TestFoldKeys(t *testing.T) {
	m := foldModel(t)
	if m.lines[1].Kind != RowFold {
		t.Fatalf("expected a fold row, got %+v", m.lines[1])
	}
	m.cursor = 1
	m.Update(key("o"))
	if len(m.lines) != len(m.rows) {
		t.Fatalf("o must unfold: %d rows", len(m.lines))
	}
	m.cursor = 2
	m.Update(key("o"))
	if m.lines[1].Kind != RowFold {
		t.Fatal("o on an unfolded removed row must fold it back")
	}
	m.Update(key("O"))
	if len(m.lines) != len(m.rows) {
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
	if strings.Contains(out, "▶+") ||
		strings.Contains(out, "▶ ") && strings.Contains(out, "│ x :=") && false {
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
	m.review.Comments = []state.Comment{
		{ID: 7, File: "a.go", Lines: "2", Severity: state.SeverityNit, Body: "rename x"},
	}
	m.rows[3] = Row{
		Kind:      RowNote,
		File:      "a.go",
		Line:      2,
		NoteKind:  "comment",
		NoteLabel: "#7 nit",
		Text:      "rename x",
		Ref:       7,
	}
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
		{
			Kind:    inbox.KindMessage,
			Step:    "s1",
			File:    "a.go",
			Lines:   "2",
			Comment: 7,
			Text:    "why nit?",
		},
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
	for _, want := range []string{
		"◇ boilerplate", "◇ generated", "◇ all changes",
	} {
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

func TestFinishButton(t *testing.T) {
	m, sent := newTestModel(t)
	var ran [][]string
	m.runGr = func(args ...string) (string, error) {
		ran = append(ran, args)
		if len(args) > 1 && args[1] == "--dry-run" {
			return "--- a.go:3\n**nit** rename\n```suggestion:-0+0\n\tx := 1\n```\n\n" +
				"--- summary\n## Guided review: approve\n" + strings.Repeat("row\n", 60), nil
		}
		return "/tmp/guided-review/mr-1\n", nil
	}
	m.Update(key("P"))
	if !strings.Contains(m.status, "nothing prepared") || len(ran) != 0 {
		t.Fatalf("P before prepare: status %q ran %v", m.status, ran)
	}
	m.review.Publish = &state.PublishPlan{Verdict: "approve"}
	if f, _ := m.footer(); !strings.Contains(ansi.Strip(f), "✓ finish · P") {
		t.Fatalf("finish button missing: %q", ansi.Strip(f))
	}
	m.Update(key("P"))
	v := ansi.Strip(m.View())
	for _, want := range []string{
		"approve · 1 comments to post · summary", "● nit     a.go:3", "│ rename", "│ ┄ suggestion", "│ +     x := 1",
		"Guided review: approve", "1–",
	} {
		if !strings.Contains(v, want) || strings.Contains(v, "\t") || strings.Contains(v, "## ") {
			t.Fatalf("finish screen lacks %q:\n%s", want, v)
		}
	}
	m.handleMouse(tea.MouseMsg{Button: tea.MouseButtonWheelDown})
	m.Update(key("G"))
	if v := ansi.Strip(m.View()); m.previewTop == 0 || !strings.Contains(v, "of 68 ") {
		t.Fatalf("finish screen must scroll to the end:\n%s", v)
	}
	_, cmd := m.Update(key("P"))
	if cmd == nil {
		t.Fatal("finishing must close the viewer")
	}
	if _, quit := cmd().(tea.QuitMsg); !quit {
		t.Fatal("finishing must close the viewer")
	}
	if len(ran) != 2 || ran[1][0] != "export" || len(ran[1]) != 1 {
		t.Fatalf("second P must run gr export: %v", ran)
	}
	want := inbox.Event{Kind: inbox.KindFinished, Step: "s1", Text: "/tmp/guided-review/mr-1"}
	if m.preview != "" || len(*sent) != 1 || (*sent)[0] != want {
		t.Fatalf("agent must get the export path: preview %q sent %+v", m.preview, *sent)
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
	m.peekFile = func(path string) []string { return numbered(20) }
	out := ansi.Strip(m.View())
	if !strings.Contains(out, "references · 2") || !strings.Contains(out, "b.go:9") ||
		!strings.Contains(out, "3 ▶ L3") {
		t.Fatalf("popup not rendered with a preview of the selected reference:\n%s", out)
	}
	m.Update(key("j"))
	if out := ansi.Strip(m.View()); !strings.Contains(out, "9 ▶ L9") {
		t.Fatalf("preview must follow the selection:\n%s", out)
	}
	m.Update(key("enter"))
	if m.popup.kind != "peek" || m.popup.loc.Path != "b.go" || len(m.popupStack) != 1 {
		t.Fatalf(
			"enter must peek the selected reference: %+v stack %d",
			m.popup,
			len(m.popupStack),
		)
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

func TestKeymapOverrides(t *testing.T) {
	km, err := newKeymap(map[string][]string{"next-hunk": {"J"}, "bogus": {"x"}})
	if err == nil || !strings.Contains(err.Error(), "unknown actions: bogus") {
		t.Fatalf("want unknown-action error, got %v", err)
	}
	m, _ := newTestModel(t)
	m.km = km
	m.Update(key("J"))
	if m.cursor != 2 {
		t.Fatalf("remapped J must jump to the next hunk, cursor %d", m.cursor)
	}
	m.Update(key("]"))
	if m.cursor != 2 {
		t.Fatal("] must be unbound after the remap")
	}
	if _, err := newKeymap(map[string][]string{"next-hunk": {"j"}}); err == nil ||
		!strings.Contains(err.Error(), "conflicts") {
		t.Fatalf("want a conflict error, got %v", err)
	}
}

func TestHelpOverlay(t *testing.T) {
	m, _ := newTestModel(t)
	m.Update(key("h"))
	out := ansi.Strip(m.View())
	for _, want := range []string{"navigate", "lsp", "gd", "go to definition", "finish"} {
		if !strings.Contains(out, want) {
			t.Fatalf("help lacks %q:\n%s", want, out)
		}
	}
	m.Update(key("x"))
	if m.help {
		t.Fatal("any key closes help")
	}
}

func TestApplyViewConfig(t *testing.T) {
	m, _ := newTestModel(t)
	m.applyConfig(
		config.Config{View: config.View{Split: true, HidePlan: true, NoMouse: true, Context: 8}},
	)
	if !m.splitView || m.showPlan || m.mouse || m.baseCtx != 8 {
		t.Fatalf(
			"config not applied: split %v plan %v mouse %v ctx %d",
			m.splitView,
			m.showPlan,
			m.mouse,
			m.baseCtx,
		)
	}
}

func TestEnterOpensFolds(t *testing.T) {
	m := foldModel(t)
	m.cursor = 1
	m.Update(key("enter"))
	if m.composing || len(m.lines) != len(m.rows) {
		t.Fatalf(
			"enter on a fold row must open it: composing %v rows %d",
			m.composing,
			len(m.lines),
		)
	}
}

func runCmd(m *model, cmd string) {
	m.Update(key(":"))
	typeText(m, cmd)
	m.Update(key("enter"))
}

func TestCommandMode(t *testing.T) {
	m, sent := newTestModel(t)
	runCmd(m, "next-hunk")
	if m.cursor != 2 {
		t.Fatalf(":next-hunk → cursor %d", m.cursor)
	}
	runCmd(m, "9")
	if m.cursor != 5 {
		t.Fatalf(":9 → cursor %d", m.cursor)
	}
	runCmd(m, "split")
	runCmd(m, "set context=7")
	if !m.splitView || m.context != 7 {
		t.Fatalf("split action and set: split %v context %d", m.splitView, m.context)
	}
	runCmd(m, "split")
	runCmd(m, "s2")
	if m.step.ID != "s2" {
		t.Fatalf(":s2 → step %s", m.step.ID)
	}
	runCmd(m, "s1")
	m.cursor = 2
	runCmd(m, "msg hello there")
	if len(*sent) != 1 || (*sent)[0].Text != "hello there" || (*sent)[0].Lines != "2" {
		t.Fatalf(":msg sent %+v", *sent)
	}
	runCmd(m, "bogus")
	if !strings.Contains(m.status, "unknown command") {
		t.Fatalf("status %q", m.status)
	}
	m.Update(key(":"))
	typeText(m, "next-h")
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if string(m.input) != "next-hunk" {
		t.Fatalf("tab completion: %q", string(m.input))
	}
	m.Update(key("esc"))
	m.Update(key(":"))
	m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if string(m.input) != "bogus" {
		t.Fatalf("history up: %q", string(m.input))
	}
	m.Update(key("esc"))
	m.Update(key(":"))
	typeText(m, "q")
	if _, cmd := m.Update(key("enter")); cmd == nil {
		t.Fatal(":q must quit")
	}
}

func TestSearch(t *testing.T) {
	m, _ := newTestModel(t)
	m.Update(key("/"))
	typeText(m, ":=")
	m.Update(key("enter"))
	if m.cursor != 2 || !strings.Contains(m.status, "match 1/3") {
		t.Fatalf("search: cursor %d status %q", m.cursor, m.status)
	}
	m.Update(key("n"))
	m.Update(key("n"))
	if m.cursor != 5 {
		t.Fatalf("n → cursor %d", m.cursor)
	}
	m.Update(key("n"))
	if m.cursor != 2 {
		t.Fatalf("n wraps → cursor %d", m.cursor)
	}
	m.Update(key("N"))
	if m.cursor != 5 {
		t.Fatalf("N wraps back → cursor %d", m.cursor)
	}
	m.Update(key("esc"))
	m.cursor = 0
	m.Update(key("n"))
	if m.cursor != 3 {
		t.Fatalf("after esc n goes to notes again, cursor %d", m.cursor)
	}
}

func TestChatPanel(t *testing.T) {
	m, _ := newTestModel(t)
	m.width = 160
	base := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	for i := range 30 {
		step := "s1"
		if i < 10 {
			step = "s0"
		}
		m.review.Messages = append(
			m.review.Messages,
			state.Message{
				Time: base.Add(time.Duration(i) * time.Minute),
				Step: step,
				Text: fmt.Sprintf("message %02d", i),
			},
		)
	}
	out := ansi.Strip(m.View())
	if !strings.Contains(out, "│ CHAT") || !strings.Contains(out, "message 29") ||
		len(m.bottomLines()) != 1 || m.mainWidth() >= m.width-m.planWidth() {
		t.Fatalf("side chat:\n%s", out)
	}
	m.handleMouse(tea.MouseMsg{X: m.width - 2, Y: 5, Button: tea.MouseButtonWheelUp})
	if m.chatTop == 0 {
		t.Fatal("wheel over the side chat must scroll it")
	}
	m.chatTop = 0
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	if m.chatTop == 0 || !strings.Contains(ansi.Strip(m.View()), "message 25") {
		t.Fatal("ctrl+y must scroll the chat up")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
	if m.chatTop != 0 {
		t.Fatal("ctrl+e must scroll the chat back down")
	}
	m.width = 90
	if n := len(m.bottomLines()); n < 3 || n > messageLines+2 {
		t.Fatalf("a narrow terminal keeps a small chat under the code: %d lines", n)
	}
	m.Update(key("c"))
	typeText(m, strings.Repeat("long words here ", 20))
	if lines := len(m.bottomLines()); lines < messageLines+3 {
		t.Fatal("long input must wrap")
	}
}

func TestKeyHints(t *testing.T) {
	m, _ := newTestModel(t)
	m.Update(key("g"))
	view := ansi.Strip(m.View())
	for _, want := range []string{
		"g…", "d  go to definition (peek)", "g  first line", "r  list references",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("hint %q missing:\n%s", want, view)
		}
	}
	m.Update(key("esc"))
	if strings.Contains(ansi.Strip(m.View()), "g…") {
		t.Fatal("hints stay after esc")
	}
}

func TestUnderlineKeepsStyles(t *testing.T) {
	styled := "\x1b[31mfoo\x1b[0m \x1b[32mbar\x1b[0m"
	got := underline(styled, 2, 7)
	if ansi.Strip(got) != "foo bar" {
		t.Fatalf("text changed: %q", ansi.Strip(got))
	}
	want := "\x1b[31mfo\x1b[4mo\x1b[0m\x1b[4m \x1b[32m\x1b[4mbar\x1b[0m\x1b[24m"
	if got != want {
		t.Fatalf("underline(%q, 2, 7) = %q, want %q", styled, got, want)
	}
}

func TestRawComment(t *testing.T) {
	m, sent := newTestModel(t)
	var ran []string
	m.runGr = func(args ...string) (string, error) {
		ran = args
		return "comment #4 nit a.go:2\n", nil
	}
	m.cursor = 2
	for _, k := range []tea.KeyMsg{key("c"), {Type: tea.KeyCtrlR}, {Type: tea.KeyTab}} {
		m.Update(k)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("-x stays, exactly")})
	if v := ansi.Strip(m.View()); !strings.Contains(v, "RAW nit a.go:2 › -x stays, exactly") {
		t.Fatalf("raw prompt missing:\n%s", v)
	}
	m.Update(key("enter"))
	want := []string{
		"comment", "add", "--file", "a.go", "--lines", "2", "--severity", "nit", "--step", "s1",
		"--", "-x stays, exactly",
	}
	if !slices.Equal(ran, want) {
		t.Fatalf("gr args = %q, want %q", ran, want)
	}
	if n := len(*sent); n == 0 || (*sent)[n-1].Kind != inbox.KindComment ||
		(*sent)[n-1].Text != "comment #4 nit a.go:2" {
		t.Fatalf("agent must be told about the raw comment: %+v", *sent)
	}

	m.composing, m.composeKind, m.composeRef, m.input = true, inbox.KindEdit, 4, []rune("reworded")
	m.Update(key("enter"))
	if want := []string{"comment", "edit", "4", "--", "reworded"}; !slices.Equal(ran, want) {
		t.Fatalf("raw edit args = %q, want %q", ran, want)
	}
}

func TestAskDeleteAndChatSize(t *testing.T) {
	m, sent := newTestModel(t)
	m.cursor = 2
	m.Update(key("?"))
	typeText(m, "why 1?")
	if v := ansi.Strip(m.View()); !strings.Contains(v, "ask a.go:2 › why 1?") {
		t.Fatalf("ask prompt missing:\n%s", v)
	}
	m.Update(key("enter"))
	want := inbox.Event{Kind: inbox.KindAsk, Step: "s1", File: "a.go", Lines: "2", Text: "why 1?"}
	if n := len(*sent); n == 0 || (*sent)[n-1] != want {
		t.Fatalf("sent %+v, want %+v", *sent, want)
	}

	var ran []string
	m.runGr = func(args ...string) (string, error) {
		ran = args
		return "comment #7 deleted\n", nil
	}
	m.lines[m.cursor].Ref = 7
	m.Update(key("D"))
	if ran != nil || !strings.Contains(m.status, "again") {
		t.Fatalf("first D must only arm: ran %q status %q", ran, m.status)
	}
	m.Update(key("j"))
	m.Update(key("k"))
	m.Update(key("D"))
	if ran != nil {
		t.Fatal("another key between the presses must disarm the delete")
	}
	m.Update(key("D"))
	if want := []string{"comment", "delete", "7"}; !slices.Equal(ran, want) {
		t.Fatalf("gr args = %q, want %q", ran, want)
	}

}

func TestBackspaceDetachesLine(t *testing.T) {
	m, sent := newTestModel(t)
	m.cursor = 2
	m.Update(key("c"))
	typeText(m, "x")
	m.Update(key("backspace"))
	if m.anchorFile == "" {
		t.Fatal("backspace that deletes text must keep the line")
	}
	m.Update(key("backspace"))
	typeText(m, "general")
	m.Update(key("enter"))
	want := inbox.Event{Kind: inbox.KindMessage, Step: "s1", Text: "general"}
	if n := len(*sent); n == 0 || (*sent)[n-1] != want {
		t.Fatalf("sent %+v, want %+v", *sent, want)
	}
}

func TestFoldNote(t *testing.T) {
	m, _ := newTestModel(t)
	i := slices.IndexFunc(m.rows, func(r Row) bool { return r.Kind == RowNote })
	m.rows[i].Text = strings.Repeat("a long remark ", 30)
	m.relist()
	notes := func() int {
		n := 0
		for _, l := range m.lines {
			if l.Kind == RowNote {
				n++
			}
		}
		return n
	}
	open := notes()
	m.seek(func(l line) bool { return l.Kind == RowNote && !l.NoteHead })
	m.Update(key("o"))
	if got := notes(); got != 1 || !strings.HasSuffix(m.current().Text, "▸") || !m.current().NoteHead {
		t.Fatalf("folded note: %d lines, cursor on %+v", got, m.current().Row)
	}
	m.Update(key("o"))
	if got := notes(); got != open || open < 2 {
		t.Fatalf("unfolded note: %d lines, want %d", got, open)
	}
}

func TestCursorHint(t *testing.T) {
	m, _ := newTestModel(t)
	footer := func() string { f, _ := m.footer(); return ansi.Strip(f) }
	if f := footer(); !strings.HasSuffix(strings.TrimSpace(f), "h help · q quit") {
		t.Fatalf("plain line footer: %q", f)
	}
	m.seek(func(l line) bool { return l.Kind == RowNote })
	if f := footer(); !strings.HasSuffix(strings.TrimSpace(f), "o fold · i details") {
		t.Fatalf("note footer: %q", f)
	}
	m.lines[m.cursor].Ref = 4
	if f := footer(); !strings.Contains(f, "E edit · DD delete · c reply · o fold") {
		t.Fatalf("comment footer: %q", f)
	}
}

func TestCountPrefix(t *testing.T) {
	m, _ := newTestModel(t)
	m.Update(key("2"))
	m.Update(key("j"))
	if m.cursor != 2 {
		t.Fatalf("2j: cursor %d, want 2", m.cursor)
	}
	for _, k := range []string{"3", "g"} {
		m.Update(key(k))
	}
	if f, _ := m.footer(); !strings.HasSuffix(strings.TrimSpace(ansi.Strip(f)), "3g") {
		t.Fatalf("typed keys not shown: %q", ansi.Strip(f))
	}
	m.Update(key("g"))
	if l := m.current(); l.File != "a.go" || l.Line != 3 {
		t.Fatalf("3gg: cursor on %s:%d", l.File, l.Line)
	}
	for _, k := range []string{"9", "G"} {
		m.Update(key(k))
	}
	if l := m.current(); l.Line != 9 {
		t.Fatalf("9G: cursor on line %d", l.Line)
	}
	m.Update(key("5"))
	m.Update(key("esc"))
	m.Update(key("k"))
	if l := m.current(); l.Line != 3 || m.count != "" {
		t.Fatalf("esc must drop the count: line %d count %q", l.Line, m.count)
	}
}

func TestWaitingForInit(t *testing.T) {
	m := &model{width: 80, height: 10, err: state.ErrNoReview}
	out := ansi.Strip(m.View())
	if !strings.Contains(out, "guided review") || !strings.Contains(out, "waiting for the agent") {
		t.Fatalf("waiting screen:\n%s", out)
	}
}

func TestChatGutter(t *testing.T) {
	m, _ := newTestModel(t)
	t0 := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	m.review.Messages = []state.Message{
		{Time: t0, Step: "s1", Text: strings.Repeat("long answer ", 12)},
		{Time: t0.Add(2 * time.Second), Step: "s1", Text: "and more"},
	}
	m.events = []inbox.Event{
		{Time: t0.Add(time.Second), Kind: inbox.KindMessage, Step: "s1", Text: "why?"},
	}
	m.lastWait = t0.Add(time.Minute)
	got := make([]string, 0)
	for _, l := range m.chatLines(60, true) {
		got = append(got, strings.TrimRight(ansi.Strip(l), " "))
	}
	want := []string{
		"── s1 first ──",
		"claude │ long answer long answer long answer long answer",
		"       │ long answer long answer long answer long answer",
		"       │ long answer long answer long answer long answer",
		"",
		"   you │ why?",
		"",
		"claude │ and more",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("chat\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	compact := m.chatLines(60, false)
	got = []string{ansi.Strip(compact[len(compact)-1])}
	if got[0] != "claude │ and more" || slices.Contains(compact, "") {
		t.Fatalf("compact chat must name speaker changes without blank lines: %q", got[0])
	}
}

func TestGeneralMessage(t *testing.T) {
	m, sent := newTestModel(t)
	m.cursor = 2
	m.Update(key("C"))
	typeText(m, "overall looks fine")
	m.Update(key("enter"))
	want := inbox.Event{Kind: inbox.KindMessage, Step: "s1", Text: "overall looks fine"}
	if len(*sent) != 1 || (*sent)[0] != want {
		t.Fatalf("sent %+v, want %+v", *sent, want)
	}
}

func TestSideChatInput(t *testing.T) {
	m, _ := newTestModel(t)
	m.width = 150
	if !strings.Contains(ansi.Strip(m.View()), "› c to write · C without a line") {
		t.Fatal("the side chat must always show where to write")
	}
	m.Update(key("C"))
	typeText(m, "hello there")
	v := ansi.Strip(m.View())
	lines := strings.Split(v, "\n")
	last := lines[len(lines)-1]
	if strings.Contains(last, "hello there") {
		t.Fatalf("input must move into the side chat, bottom line is %q", last)
	}
	i := slices.IndexFunc(lines, func(l string) bool { return strings.Contains(l, "› hello there") })
	if i < 0 || strings.Index(lines[i], "› hello there") < m.width-m.chatWidth() {
		t.Fatalf("input must sit in the right column:\n%s", v)
	}
	m.Update(key("esc"))
	m.Update(key(":"))
	v = ansi.Strip(m.View())
	if lines := strings.Split(v, "\n"); !strings.HasPrefix(lines[len(lines)-1], ":") {
		t.Fatalf("command line stays at the bottom:\n%s", v)
	}
}

func TestDragResize(t *testing.T) {
	m, _ := newTestModel(t)
	m.width = 200
	m.review.Messages = []state.Message{{Step: "s1", Text: "hi"}}
	drag := func(fromX, fromY, toX, toY int) {
		m.handleMouse(tea.MouseMsg{X: fromX, Y: fromY, Action: tea.MouseActionPress,
			Button: tea.MouseButtonLeft})
		m.handleMouse(tea.MouseMsg{X: toX, Y: toY, Action: tea.MouseActionMotion,
			Button: tea.MouseButtonLeft})
		m.handleMouse(tea.MouseMsg{X: toX, Y: toY, Action: tea.MouseActionRelease})
	}
	drag(m.planWidth()-1, 3, 45, 3)
	if got := m.planWidth(); got != 46 {
		t.Fatalf("plan width after drag = %d, want 46", got)
	}
	drag(m.width-m.chatWidth(), 3, m.width-60, 3)
	if got := m.chatWidth(); got != 60 {
		t.Fatalf("side chat width after drag = %d, want 60", got)
	}
	if m.visual || m.resizing != "" {
		t.Fatal("resizing must not select lines and must stop on release")
	}
}

func TestMouseOffIsVisible(t *testing.T) {
	m, _ := newTestModel(t)
	m.mouse = false
	if f, _ := m.footer(); !strings.Contains(ansi.Strip(f), "mouse off · m") {
		t.Fatalf("footer must say the mouse is off: %q", ansi.Strip(f))
	}
}

func TestNoteDetails(t *testing.T) {
	m, sent := newTestModel(t)
	m.seek(func(l line) bool { return l.Kind == RowNote })
	note := m.current()
	m.Update(key("i"))
	if m.popup == nil || m.popup.kind != "detail" ||
		!strings.Contains(ansi.Strip(strings.Join(m.popup.lines, "")), "writing the details") {
		t.Fatalf("i must open a waiting popup: %+v", m.popup)
	}
	want := inbox.Event{Kind: inbox.KindDetail, Step: "s1", File: note.File,
		Lines: fmt.Sprint(note.Line), Text: "why x"}
	if n := len(*sent); n != 1 || (*sent)[0] != want {
		t.Fatalf("sent %+v, want %+v", *sent, want)
	}
	m.step.Details = []state.Detail{{File: note.File, Line: note.Line,
		Text: "x is the **fee**.\n```go\nx := fee(a)\n```"}}
	m.refreshDetail()
	body := ansi.Strip(strings.Join(m.popup.lines, "\n"))
	if !strings.Contains(body, "x is the fee.") || !strings.Contains(body, "x := fee(a)") {
		t.Fatalf("popup must show the detail:\n%s", body)
	}
	m.Update(key("esc"))
	m.Update(key("i"))
	if len(*sent) != 1 || !strings.Contains(ansi.Strip(strings.Join(m.popup.lines, "\n")), "fee") {
		t.Fatal("a stored detail opens without asking the agent again")
	}
}

func TestLocalNext(t *testing.T) {
	m, sent := newTestModel(t)
	m.review.Steps[0].Message = "s1: guard"
	m.review.Steps[0].Hotspots = []state.Hotspot{{Cat: "money", Q: "rounding?"}}
	var ran [][]string
	out := "step s2\n"
	m.runGr = func(args ...string) (string, error) { ran = append(ran, args); return out, nil }
	m.Update(key(">"))
	if len(ran) != 0 || !strings.Contains(m.status, "risk question") {
		t.Fatalf("first > on a hotspot step must only remind: ran %v status %q", ran, m.status)
	}
	m.Update(key(">"))
	if len(ran) != 1 || !slices.Equal(ran[0], []string{"step", "next"}) || len(*sent) != 0 {
		t.Fatalf("second > must move without the agent: ran %v sent %+v", ran, *sent)
	}
	m.Update(key("S"))
	typeText(m, "trivial")
	m.Update(key("enter"))
	if want := []string{"step", "skip", "--reason", "trivial"}; !slices.Equal(ran[1], want) {
		t.Fatalf("skip ran %v, want %v", ran[1], want)
	}
	out = "all steps reviewed: run gr status\n"
	m.review.Steps[0].Hotspots = nil
	m.View()
	m.Update(key(">"))
	if n := len(*sent); n != 1 || (*sent)[0].Kind != inbox.KindReviewed {
		t.Fatalf("the agent must hear when all steps are reviewed: %+v", *sent)
	}
}

func TestDetailPopupUX(t *testing.T) {
	m, _ := newTestModel(t)
	m.height = 16
	for i := range 40 {
		m.rows = append(m.rows, Row{Kind: RowCode, File: "a.go", Line: 20 + i, Text: "x"})
	}
	m.rows = append(m.rows, Row{
		Kind: RowNote, File: "a.go", Line: 59, NoteKind: "note", Text: "late note",
	})
	m.relist()
	m.seek(func(l line) bool { return l.Kind == RowNote && l.Line == 59 })
	m.clamp()
	m.Update(key("i"))
	detail := strings.Repeat("detail line\n", 30)
	m.step.Details = []state.Detail{{File: "a.go", Line: 59, Text: detail}}
	m.refreshDetail()
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "late note") || strings.Contains(v, "e editor") {
		t.Fatalf("the noted line must stay above the popup, without an editor hint:\n%s", v)
	}
	m.handleMouse(tea.MouseMsg{Button: tea.MouseButtonWheelDown})
	if m.popup.top == 0 {
		t.Fatal("the wheel must scroll the popup")
	}
}

func TestClickChatInput(t *testing.T) {
	m, _ := newTestModel(t)
	m.width = 150
	bodyH := m.height - len(m.bottomLines())
	m.handleMouse(tea.MouseMsg{X: m.width - 5, Y: bodyH - 1, Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft})
	if !m.composing || m.composeKind != inbox.KindMessage {
		t.Fatal("a click on the side chat's input line must open the input")
	}

	r := &state.Review{ID: "mr-1"}
	intake := &model{review: r, width: 100, height: 20, send: func(inbox.Event) error { return nil }}
	intake.handleMouse(tea.MouseMsg{X: 30, Y: 18, Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft})
	if !intake.composing {
		t.Fatal("a click on the intake answer field must open the input")
	}
}

func TestAnswerOptions(t *testing.T) {
	t0 := time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)
	var sent []inbox.Event
	r := &state.Review{ID: "mr-1", Messages: []state.Message{
		{Time: t0, Text: "Верно понял?", Options: []string{"да", "нет, поправлю"}},
	}}
	m := &model{review: r, width: 100, height: 20,
		send: func(e inbox.Event) error { sent = append(sent, e); return nil }}
	if v := ansi.Strip(m.View()); !strings.Contains(v, " 1 да   2 нет, поправлю") {
		t.Fatalf("intake must offer the answers:\n%s", v)
	}
	m.Update(key("2"))
	if len(sent) != 1 || sent[0].Text != "нет, поправлю" {
		t.Fatalf("2 must send the second answer: %+v", sent)
	}
	m.events = []inbox.Event{{Time: t0.Add(time.Second), Kind: inbox.KindMessage, Text: "x"}}
	if len(m.answerOptions()) != 0 {
		t.Fatal("answers disappear once the human replied")
	}

	s, sentS := newTestModel(t)
	s.review.Messages = []state.Message{{Time: t0, Step: "s1", Text: "?", Options: []string{"да"}}}
	s.Update(key("1"))
	if len(*sentS) != 0 {
		t.Fatal("during steps a bare digit is a count, not an answer")
	}
	s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("1"), Alt: true})
	if len(*sentS) != 1 || (*sentS)[0].Text != "да" {
		t.Fatalf("alt+1 must answer: %+v", *sentS)
	}
}

func TestLSPFromPeek(t *testing.T) {
	m, _ := newTestModel(t)
	var asked []string
	m.lspDo = func(kind, file string, line, col int) tea.Cmd {
		asked = append(asked, fmt.Sprintf("%s %s:%d:%d", kind, file, line, col))
		return nil
	}
	m.peekFile = func(string) []string { return []string{"package b", "func B() { helper() }"} }
	m.openPeek(lspLoc{Path: "b.go", Line: 1})
	for _, k := range []string{"j", "w", "w", "g", "i"} {
		m.Update(key(k))
	}
	if len(asked) != 1 || asked[0] != "implementation b.go:2:11" {
		t.Fatalf("gi in the peek must ask about the word under its cursor: %q", asked)
	}
	first := m.popup
	m.handleLSP(lspMsg{kind: "definition", locs: []lspLoc{{Path: "c.go", Line: 7}}})
	if m.popup == first || m.popup.loc.Path != "c.go" || len(m.popupStack) != 1 {
		t.Fatalf("a result from the peek opens on top of it: stack %d, %+v", len(m.popupStack), m.popup)
	}
	m.Update(key("esc"))
	if m.popup != first {
		t.Fatal("esc must go back to the previous peek")
	}
	for _, k := range []string{"esc"} {
		m.Update(key(k))
	}
	m.seek(func(l line) bool { return l.Kind == RowCode })
	for _, seq := range [][]string{{"g", "y"}, {"g", "c"}} {
		for _, k := range seq {
			m.Update(key(k))
		}
	}
	if len(asked) != 3 || !strings.HasPrefix(asked[1], "typeDefinition ") ||
		!strings.HasPrefix(asked[2], "callers ") {
		t.Fatalf("gy and gc must ask the server: %q", asked)
	}
}

func TestDiagnosticsAndSymbols(t *testing.T) {
	m, _ := newTestModel(t)
	var asked []string
	m.lspDo = func(kind, file string, line, col int) tea.Cmd {
		asked = append(asked, kind)
		return nil
	}
	if _, cmd := m.Update(key("j")); cmd == nil || m.diagStep != "s1" {
		t.Fatal("opening a step must start collecting its diagnostics")
	}
	m.Update(diagMsg{step: "s2", diags: map[string][]lsp.Diagnostic{"a.go": {{Line: 1, Severity: 1}}}})
	if m.diags != nil {
		t.Fatal("diagnostics of another step are dropped")
	}
	m.Update(diagMsg{step: "s1", diags: map[string][]lsp.Diagnostic{"a.go": {
		{Line: 1, Severity: 1, Message: "x declared and not used", Source: "compiler"},
		{Line: 2, Severity: 3, Message: "hint"},
	}}})
	var diag []Note
	for _, n := range m.notes() {
		if n.Kind == "error" || n.Kind == "warning" {
			diag = append(diag, n)
		}
	}
	want := Note{File: "a.go", Line: 2, Kind: "error", Text: "x declared and not used (compiler)"}
	if len(diag) != 1 || diag[0] != want {
		t.Fatalf("diagnostic notes %+v, want only %+v", diag, want)
	}
	m.cursor = 2
	m.Update(key(":"))
	typeText(m, "sym Transfer")
	m.Update(key("enter"))
	if len(asked) != 1 || asked[0] != "workspace:Transfer" {
		t.Fatalf(":sym must search the workspace: %q", asked)
	}
}

func TestSeenLines(t *testing.T) {
	m, _ := newTestModel(t)
	m.review.Steps[0].Message = "s1"
	var ran int
	m.runGr = func(...string) (string, error) { ran++; return "", nil }
	if m.unseen() == 0 {
		t.Fatal("nothing is seen before the step is on screen")
	}
	m.Update(key(">"))
	if ran != 0 || !strings.Contains(m.status, "changed lines not seen yet") {
		t.Fatalf("> must warn about unseen lines: ran %d status %q", ran, m.status)
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "✓ seen") || m.unseen() != 0 {
		t.Fatalf("rendering the whole step marks it seen:\n%s", v)
	}
	m.confirmNext = ""
	m.Update(key(">"))
	if ran != 1 {
		t.Fatal("a fully seen step moves on the first >")
	}
}

func TestChapterIntro(t *testing.T) {
	m, _ := newTestModel(t)
	m.step.Chapter = "Переводы"
	m.step.Intro = "Before: a retry debited twice. After: the key returns the first result."
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "▌ Переводы") || !strings.Contains(v, "▌ Before: a retry debited twice.") {
		t.Fatalf("the chapter intro must head its first step:\n%s", v)
	}
	m.review.Steps[1].Chapter = "Переводы"
	m.step = &m.review.Steps[1]
	if v := ansi.Strip(m.View()); !strings.Contains(v, "▌ Переводы · Before: a retry debited twice.") {
		t.Fatalf("later steps of the chapter keep a one-line reminder:\n%s", v)
	}
}

func TestFlowSection(t *testing.T) {
	m, _ := newTestModel(t)
	m.height = 40
	m.Update(flowMsg{step: "s2", entries: []flowEntry{{name: "ignored"}}})
	if m.flow != nil {
		t.Fatal("flow of another step is dropped")
	}
	m.Update(flowMsg{step: "s1", entries: []flowEntry{
		{name: "reserve", in: []string{"Handle", "Retry"}, out: []string{"Insert"}},
	}})
	v := ansi.Strip(m.View())
	for _, want := range []string{"FLOW", "reserve", "← Handle, Retry", "→ Insert"} {
		if !strings.Contains(v, want) {
			t.Fatalf("plan pane lacks %q:\n%s", want, v)
		}
	}
}

func TestInterruptAgent(t *testing.T) {
	m, sent := newTestModel(t)
	var tmux [][]string
	m.tmux = func(args ...string) error { tmux = append(tmux, args); return nil }
	m.agentWaiting = true
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if len(tmux) != 0 || m.composing {
		t.Fatal("ctrl+c does nothing while it is the human's turn")
	}
	m.agentWaiting, m.returnPane = false, "%11"
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC}); cmd != nil {
		if _, quit := cmd().(tea.QuitMsg); quit {
			t.Fatal("ctrl+c must not quit the viewer")
		}
	}
	if len(tmux) != 1 || !slices.Equal(tmux[0], []string{"send-keys", "-t", "%11", "Escape"}) ||
		!m.composing {
		t.Fatalf("ctrl+c must interrupt the agent and open the input: %q", tmux)
	}
	typeText(m, "and check the rollback too")
	m.Update(key("enter"))
	if n := len(*sent); n != 1 || (*sent)[0].Text != "and check the rollback too" {
		t.Fatalf("the message goes through the inbox: %+v", *sent)
	}
	want := [][]string{
		{"send-keys", "-t", "%11", "-l", resumeAgent},
		{"send-keys", "-t", "%11", "Enter"},
	}
	if len(tmux) != 3 || !slices.Equal(tmux[1], want[0]) || !slices.Equal(tmux[2], want[1]) {
		t.Fatalf("the agent must be told to go on: %q", tmux)
	}
}

func TestYank(t *testing.T) {
	m, _ := newTestModel(t)
	var got string
	m.clip = func(text string) error { got = text; return nil }
	m.cursor = 1
	m.Update(key("v"))
	m.Update(key("j"))
	m.Update(key("y"))
	if got != "package a\nx := 1" || m.visual || !strings.Contains(m.status, "copied 2 lines") {
		t.Fatalf("y copied %q, status %q", got, m.status)
	}
}
