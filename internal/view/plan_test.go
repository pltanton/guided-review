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
	if !strings.Contains(lines[1], "● ⚑1 1") || !strings.Contains(lines[2], "● 2") {
		t.Fatalf("order, flags and the unseen dot:\n%s", got)
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

func TestReadingBlockWhileTheAgentReads(t *testing.T) {
	m, _ := twoFileModel(t)
	m.width = 160
	m.review.Files[1].Read, m.review.Files[2].Read = false, false
	m.review.Progress = &state.Progress{Text: "reading b.go"}
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "read 1/3") || !strings.Contains(v, "■■■") || !strings.Contains(v, "reading b.go") {
		t.Fatalf("the block shows the flag, a square per file and the progress:\n%s", v)
	}
	m.review.Files[1].Read, m.review.Files[2].Read = true, true
	if v := ansi.Strip(m.View()); !strings.Contains(v, "read 3/3") {
		t.Fatalf("the block stays while the agent works:\n%s", v)
	}
	m.agentIdle = true
	if v := ansi.Strip(m.View()); strings.Contains(v, "agent at work") {
		t.Fatal("a stopped agent shows no block")
	}
	m.agentIdle, m.agentWaiting = false, true
	if v := ansi.Strip(m.View()); strings.Contains(v, "agent at work") {
		t.Fatal("your turn shows no block")
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
	m.Update(key("j"))
	m.Update(key("j"))
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
