package view

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/pltanton/guided-review/internal/gitx"
	"github.com/pltanton/guided-review/internal/state"
	"github.com/pltanton/guided-review/internal/testrepo"
)

func treeText(m *model) string {
	row, plain, selected := sideRows(40)
	entries, _ := m.fileEntries(row, plain, selected)
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
