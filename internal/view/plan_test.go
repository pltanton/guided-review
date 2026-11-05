package view

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/pltanton/guided-review/internal/gitx"
	"github.com/pltanton/guided-review/internal/inbox"
	"github.com/pltanton/guided-review/internal/state"
	"github.com/pltanton/guided-review/internal/testrepo"
)

func treeText(m *model) string {
	row, plain, selected := sideRows(40)
	entries, _ := m.fileEntries(40, row, plain, selected)
	var out []string
	for _, e := range entries {
		out = append(out, ansi.Strip(e.text))
	}
	return strings.Join(out, "\n")
}

func TestTreeOrderAndMarks(t *testing.T) {
	m, _ := twoFileModel(t)
	m.review.Files = []state.File{
		{Path: "a.go", Tier: state.TierCore},
		{Path: "api/b.go", Tier: state.TierCore, Order: 2, Read: true},
		{Path: "api/c.go", Tier: state.TierCore, Order: 1, Read: true},
		{Path: "gen.pb.go", Tier: state.TierGenerated},
		{Path: "wire.go", Tier: state.TierBoilerplate},
	}
	m.review.Hotspots = []state.Hotspot{{Cat: "money", Q: "?", File: "api/c.go", Line: 7}}
	m.width = 160
	m.relist()
	got := treeText(m)
	lines := strings.Split(got, "\n")
	want := []string{"api/", "c.go", "b.go", "a.go", "▸ boilerplate", "▸ generated"}
	for i, w := range want {
		if i >= len(lines) || !strings.Contains(lines[i], w) {
			t.Fatalf("tree row %d lacks %q:\n%s", i, w, got)
		}
	}
	if !strings.Contains(lines[1], "+0   −0 ● ⚑1  1") || !strings.Contains(lines[2], "●     2") ||
		!strings.Contains(lines[4], "▸ boilerplate 1") {
		t.Fatalf("stats, order, flags and the unseen dot:\n%s", got)
	}
	m.View()
	if strings.Contains(treeText(m), "●") {
		t.Fatalf("after the change was on screen no dot stays:\n%s", treeText(m))
	}
	items := m.treeItems()
	if len(items) != 5 || items[3].file != "" || items[3].tier != state.TierBoilerplate {
		t.Fatalf("treeItems = %+v", items)
	}
}

func TestFoldedTiersOpenOnEnter(t *testing.T) {
	m, _ := newTestModel(t)
	m.review.Files = append(m.review.Files, state.File{Path: "wire.go", Tier: state.TierBoilerplate})
	tr := testrepo.New(t)
	tr.Write("a.go", "package a\n")
	base := tr.Commit("base")
	tr.Write("a.go", "package a\n\nvar X = 1\n")
	tr.Write("wire.go", "package a\n")
	head := tr.Commit("head")
	m.src = newGitSource(context.Background(), gitx.Repo{Dir: tr.Dir}, gitx.DefaultDiffAlgorithm, base, head)
	m.width = 160
	m.rebuild(true)
	if strings.Contains(ansi.Strip(m.View()), "▍ wire.go") {
		t.Fatal("a boilerplate file is folded away at first")
	}
	m.Update(key("f"))
	m.Update(key("G"))
	m.Update(key("enter"))
	if !m.tierOpen[state.TierBoilerplate] || !strings.Contains(ansi.Strip(m.View()), "▍ wire.go") {
		t.Fatalf("enter on the group opens it:\n%s", ansi.Strip(m.View()))
	}
	m.Update(key("enter"))
	if m.tierOpen[state.TierBoilerplate] || strings.Contains(ansi.Strip(m.View()), "▍ wire.go") {
		t.Fatal("enter on the group row folds it again")
	}
	m.review.Files[0].Order, m.review.Files[1].Tier, m.review.Files[1].Order = 2, state.TierCore, 1
	m.rebuild(true)
	if files := m.shownFiles(); !slices.Equal(files, []string{"wire.go", "a.go"}) {
		t.Fatalf("the diff follows the agent's order: %v", files)
	}
}

func TestFilesScroll(t *testing.T) {
	m, _ := twoFileModel(t)
	for i := range 20 {
		f := fmt.Sprintf("pkg/f%02d.go", i)
		m.review.Files = append(m.review.Files, state.File{Path: f, Tier: state.TierCore, Read: true})
		m.rows = append(m.rows, Row{Kind: RowFile, File: f, Text: f},
			Row{Kind: RowAdded, File: f, Line: 1, Text: "x", HunkStart: true})
	}
	m.width, m.height = 160, 20
	m.relist()
	m.Update(key("f"))
	for range 18 {
		m.Update(key("j"))
	}
	side := ansi.Strip(m.View())
	if !strings.Contains(side, "f15.go") || !strings.Contains(side, "↑") {
		t.Fatalf("files must scroll to the cursor:\n%s", side)
	}
	m.Update(key("enter"))
	if m.current().File != "pkg/f15.go" {
		t.Fatalf("enter in files: %+v", m.current())
	}
}

func TestFilesModalWhenNarrow(t *testing.T) {
	m, _ := twoFileModel(t)
	m.width = 80
	m.relist()
	m.Update(key("f"))
	v := ansi.Strip(m.View())
	if m.chatWidth() != 0 || !strings.Contains(v, "┌ files") || !strings.Contains(v, "c.go") {
		t.Fatalf("narrow: f opens the files modal:\n%s", v)
	}
	m.Update(key("j"))
	m.Update(key("j"))
	m.Update(key("enter"))
	if m.focusFiles || m.current().File != "api/c.go" {
		t.Fatalf("enter in the files modal jumps: focus %v file %s", m.focusFiles, m.current().File)
	}
}

func TestWholeFilePeek(t *testing.T) {
	m, _ := newTestModel(t)
	m.peekFile = func(string) []string { return numbered(20) }
	m.cursor = 4
	m.Update(key("F"))
	if m.popup == nil || m.popup.kind != "peek" || m.popup.loc.Path != "a.go" || m.popup.loc.Line != 3 ||
		m.popup.title != "a.go:3" {
		t.Fatalf("F opens the file at the cursor line: %+v", m.popup)
	}
	m.Update(key("esc"))
	if m.popup != nil {
		t.Fatal("esc closes it")
	}
}

func TestUnreadFilesSaySoOnTheirHeader(t *testing.T) {
	m, _ := twoFileModel(t)
	m.width = 160
	m.rows[0].Unread = true
	m.relist()
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "the agent has not read this yet") || strings.Count(v, "not read") != 1 {
		t.Fatalf("an unread file's header says the agent has not read it, the others say nothing:\n%s", v)
	}
	m.agentIdle = true
	if v := ansi.Strip(m.View()); !strings.Contains(v, "○ not read by the agent") {
		t.Fatalf("with the agent stopped the header says so without a spinner:\n%s", v)
	}
	m.review.Files[1].Read = false
	if side := treeText(m); !strings.Contains(side, "b.go") || strings.Count(side, "○") < 1 {
		t.Fatalf("the tree marks an unread file too:\n%s", side)
	}
}

func TestOrderArrivalMovesAReaderStillAtTheStart(t *testing.T) {
	m, _ := twoFileModel(t)
	store := state.Store{Dir: t.TempDir()}
	tr := testrepo.New(t)
	tr.Write("a.go", "package a\n")
	tr.Write("api/b.go", "package api\n")
	tr.Write("api/c.go", "package api\n")
	base := tr.Commit("base")
	tr.Write("a.go", "package a\n\nvar A = 1\n")
	tr.Write("api/b.go", "package api\n\nvar B = 1\n")
	tr.Write("api/c.go", "package api\n\nvar C = 1\n")
	head := tr.Commit("head")
	m.review.BaseSHA, m.review.HeadSHA = base, head
	m.repo = gitx.Repo{Dir: tr.Dir}
	if err := store.Save(m.review); err != nil {
		t.Fatal(err)
	}
	if err := store.SetCurrent(m.review.ID); err != nil {
		t.Fatal(err)
	}
	m.store, m.review, m.src, m.algo, m.ctx = store, nil, nil, gitx.DefaultDiffAlgorithm, context.Background()
	m.reload()
	if m.err != nil || m.current().File != "a.go" {
		t.Fatalf("path order first: %+v", m.current())
	}
	err := store.UpdateCurrent(func(r *state.Review) error {
		r.File("api/c.go").Order = 1
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	m.reload()
	if m.current().File != "api/c.go" || m.offset != 0 {
		t.Fatalf("a reader still at the start follows the order: %+v offset %d", m.current(), m.offset)
	}
	m.seek(func(l line) bool { return l.File == "api/b.go" && l.Line == 3 })
	was := m.current()
	err = store.UpdateCurrent(func(r *state.Review) error {
		r.File("a.go").Order = 1
		r.File("api/c.go").Order = 2
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	m.reload()
	if m.current().File != was.File || m.current().Line != was.Line {
		t.Fatalf("a reader who moved keeps the place: %+v, was %+v", m.current(), was)
	}
}

func TestNoScrollBeforeTheSizeIsKnown(t *testing.T) {
	m, _ := twoFileModel(t)
	m.width, m.height = 0, 0
	m.cursor = 6
	m.relist()
	if m.offset != 0 {
		t.Fatalf("without a size the view stays at the top, offset %d", m.offset)
	}
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if m.offset != 0 || m.cursor != 6 {
		t.Fatalf("the first size keeps the top in view: offset %d cursor %d", m.offset, m.cursor)
	}
}

func TestThreadOnAFileOutsideTheRoundSitsOnTop(t *testing.T) {
	m, _ := newTestModel(t)
	m.review.MR = &state.MR{Me: "me"}
	m.review.Round = 2
	m.review.Discussions = []state.Discussion{{ID: "d1", Author: "me", Resolvable: true,
		File: "gone/elsewhere.go", Line: 7, Body: "is this safe?",
		Notes: []state.Note{{Author: "me", Body: "is this safe?"}, {Author: "kai", Body: "yes, fixed"}}}}
	notes := m.notes()
	var top []Note
	for _, n := range notes {
		if n.Thread == "d1" {
			top = append(top, n)
		}
	}
	if len(top) == 0 {
		t.Fatal("the thread is shown")
	}
	for _, n := range top {
		if !n.Top || n.File != "" {
			t.Fatalf("a thread on a file outside the round is a top note: %+v", n)
		}
	}
	if !strings.Contains(top[0].Text, "elsewhere.go:7 (not in this round)") {
		t.Fatalf("the first note says where the thread is: %q", top[0].Text)
	}
}

func TestFocusSurvivesARebuildOnRemovedLinesAndNotes(t *testing.T) {
	m, _ := newTestModel(t)
	m.rows = append(m.rows,
		Row{Kind: RowRemoved, File: "a.go", OldLine: 7, Text: "gone"},
		Row{Kind: RowRemoved, File: "a.go", OldLine: 8, Text: "gone too"},
		Row{Kind: RowNote, File: "a.go", Line: 9, Text: "first", NoteKind: "mr", NoteLabel: "@kai"},
		Row{Kind: RowNote, File: "a.go", Line: 9, Text: "second", NoteKind: "hotspot", NoteLabel: "DECIDE"},
	)
	m.showRemoved = true
	m.relist()
	m.seek(func(l line) bool { return l.Kind == RowRemoved && l.OldLine == 8 })
	keep := m.current()
	m.rows = append([]Row{{Kind: RowNote, File: "a.go", Line: 1, Text: "new on top", NoteKind: "note"}}, m.rows...)
	m.relist()
	if cur := m.current(); cur.Kind != RowRemoved || cur.OldLine != 8 {
		t.Fatalf("a removed line keeps the cursor after a rebuild: %+v (was %+v)", cur.Row, keep.Row)
	}
	m.seek(func(l line) bool { return l.NoteKind == "hotspot" && l.NoteHead })
	m.rows = append([]Row{{Kind: RowNote, File: "a.go", Line: 1, Text: "another", NoteKind: "spec"}}, m.rows...)
	m.relist()
	if cur := m.current(); cur.NoteKind != "hotspot" || !cur.NoteHead {
		t.Fatalf("the same note keeps the cursor, not the first note on its line: %+v", cur.Row)
	}
}

func TestReplyOnAnotherReviewersThread(t *testing.T) {
	m, sent := newTestModel(t)
	m.store = state.Store{Dir: t.TempDir()}
	m.review.MR = &state.MR{Me: "me"}
	m.review.Discussions = []state.Discussion{{ID: "d1", Author: "bob", Resolvable: true, File: "a.go", Line: 2,
		Body: "why not an error?"}}
	if err := m.store.Save(m.review); err != nil {
		t.Fatal(err)
	}
	if err := m.store.SetCurrent(m.review.ID); err != nil {
		t.Fatal(err)
	}
	m.rows = append(m.rows, Row{Kind: RowNote, File: "a.go", Line: 2, Text: "why not an error?",
		NoteKind: "mr", NoteLabel: "@bob", Thread: "d1"})
	m.relist()
	m.seek(func(l line) bool { return l.Thread == "d1" })
	m.Update(key("enter"))
	v := ansi.Strip(m.View())
	for _, want := range []string{" 1  reply to the author", " 2  ask the agent for a reply, with directions",
		" 3  discuss it with the agent"} {
		if !strings.Contains(v, want) {
			t.Fatalf("another reviewer's thread offers a reply:\n%s", v)
		}
	}
	m.Update(key("2"))
	typeText(m, "say it is a nit and we keep the panic")
	m.Update(key("enter"))
	if n := len(*sent); n != 1 || (*sent)[0].Kind != inbox.KindReply || (*sent)[0].Thread != "d1" ||
		(*sent)[0].Text != "say it is a nit and we keep the panic" {
		t.Fatalf("directions reach the agent as a reply request: %+v", *sent)
	}
	if got := (*sent)[0].String(); got != "[reply] thread d1: say it is a nit and we keep the panic" {
		t.Fatalf("event line: %q", got)
	}
	err := m.store.UpdateCurrent(func(r *state.Review) error {
		r.SetThread(state.Thread{ID: "d1", Proposed: state.VerdictOpen, ProposedReply: "A nit: the panic stays, it is unreachable."})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	m.reload()
	m.seek(func(l line) bool { return l.Thread == "d1" })
	m.Update(key("enter"))
	if v := ansi.Strip(m.View()); !strings.Contains(v, " 3  send the reply the agent suggests") {
		t.Fatalf("the agent's draft becomes an item:\n%s", v)
	}
	m.Update(key("3"))
	r, err := m.store.LoadCurrent()
	if err != nil {
		t.Fatal(err)
	}
	if th := r.ThreadState(r.Discussions[0]); !th.Decided() || th.Reply != "A nit: the panic stays, it is unreachable." {
		t.Fatalf("the reply is recorded for publishing: %+v", th)
	}
	if label, _ := m.threadBadge(m.review.Discussions[0]); label != "@bob ↩ your reply" {
		t.Fatalf("the row shows the reply: %q", label)
	}
}

func TestThreadPastTheEndOfTheFileSitsOnTop(t *testing.T) {
	m, _ := newTestModel(t)
	tr := testrepo.New(t)
	tr.Write("a.go", "package a\n")
	base := tr.Commit("base")
	tr.Write("a.go", strings.Repeat("x\n", 30))
	head := tr.Commit("head")
	m.src = newGitSource(context.Background(), gitx.Repo{Dir: tr.Dir}, gitx.DefaultDiffAlgorithm, base, head)
	m.review.MR = &state.MR{Me: "me"}
	m.review.Discussions = []state.Discussion{
		{ID: "gone", Author: "me", Resolvable: true, File: "a.go", Line: 47, Body: "still needed?"},
		{ID: "here", Author: "me", Resolvable: true, File: "a.go", Line: 10, Body: "and this?"},
	}
	top := map[string]bool{}
	for _, n := range m.notes() {
		if n.Thread != "" {
			top[n.Thread] = top[n.Thread] || n.Top
			if n.Thread == "gone" && n.Top && !strings.Contains(n.Text, "a.go:47 (the line is gone)") &&
				!strings.HasPrefix(n.Text, "agent") && n.Kind == "comment" {
				t.Fatalf("the first note says the line is gone: %q", n.Text)
			}
		}
	}
	if !top["gone"] || top["here"] {
		t.Fatalf("a thread past the file's end sits on top, one inside stays on its line: %v", top)
	}
}

func sinceModel(t *testing.T) *model {
	t.Helper()
	m, _ := twoFileModel(t)
	m.review.Files = []state.File{
		{Path: "a.go", Tier: state.TierCore, Added: 2, Deleted: 0, Read: true},
		{Path: "api/b.go", Tier: state.TierCore, Added: 1, Deleted: 0, Read: true},
	}
	tr := testrepo.New(t)
	tr.Write("a.go", "package a\n")
	base := tr.Commit("base")
	tr.Write("a.go", "package a\n\nvar A = 1\n")
	tr.Write("api/b.go", "package api\n")
	tr.Commit("first: A and b")
	tr.Write("a.go", "package a\n\nvar A = 2\n")
	head := tr.Commit("second: A again")
	m.review.BaseSHA, m.review.HeadSHA = base, head
	m.repo, m.ctx, m.algo = gitx.Repo{Dir: tr.Dir}, context.Background(), gitx.DefaultDiffAlgorithm
	store := state.Store{Dir: t.TempDir()}
	if err := store.Save(m.review); err != nil {
		t.Fatal(err)
	}
	if err := store.SetCurrent(m.review.ID); err != nil {
		t.Fatal(err)
	}
	m.store, m.review, m.src, m.width = store, nil, nil, 160
	m.reload()
	return m
}

func TestSinceShowsTheLastCommitsOnly(t *testing.T) {
	m := sinceModel(t)
	if !slices.Equal(m.shownFiles(), []string{"a.go", "api/b.go"}) {
		t.Fatalf("the whole change first: %v", m.shownFiles())
	}
	m.Update(key(":"))
	typeText(m, "since 1")
	m.Update(key("enter"))
	if m.since == "" || m.sinceN != 1 || !slices.Equal(m.shownFiles(), []string{"a.go", ""}) {
		t.Fatalf(":since 1 keeps only the files of the last commit: %v, since %q", m.shownFiles(), m.since)
	}
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "since 1 commit (") || !strings.Contains(v, "▸ unch") ||
		!strings.Contains(v, "+1   −1") {
		t.Fatalf("the header names the scope, the tree folds the unchanged file and counts the scope:\n%s", v)
	}
	m.Update(key(":"))
	typeText(m, "since")
	m.Update(key("enter"))
	if m.since != "" || !slices.Equal(m.shownFiles(), []string{"a.go", "api/b.go"}) {
		t.Fatalf(":since alone shows the whole change again: %v", m.shownFiles())
	}
}

func TestCommitCardPicksAScope(t *testing.T) {
	m := sinceModel(t)
	m.Update(key("L"))
	v := ansi.Strip(m.View())
	for _, want := range []string{"┌ commits", " 1  whole change", " 2  ", "second: A again", " 3  ", "first: A and b"} {
		if !strings.Contains(v, want) {
			t.Fatalf("the card lists the commits newest first:\n%s", v)
		}
	}
	m.Update(key("3"))
	if m.commitCard || m.since != "" || !slices.Equal(m.shownFiles(), []string{"a.go", "api/b.go"}) {
		t.Fatalf("picking the oldest commit is the whole change: %q files %v", m.since, m.shownFiles())
	}
	m.Update(key("L"))
	m.Update(key("2"))
	if m.sinceN != 1 || !slices.Equal(m.shownFiles(), []string{"a.go", ""}) {
		t.Fatalf("picking the newest commit shows it alone, the rest folded: n %d files %v", m.sinceN, m.shownFiles())
	}
	m.Update(key("L"))
	if m.commitSel != 1 {
		t.Fatalf("the card marks the current pick: %d", m.commitSel)
	}
	m.Update(key("1"))
	if m.since != "" {
		t.Fatal("whole change resets the scope")
	}
}
