package view

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/pltanton/guided-review/internal/state"
)

func bigPlanModel(t *testing.T, current int) *model {
	t.Helper()
	m, _ := newTestModel(t)
	r := &state.Review{ID: "mr-1"}
	for i := range 76 {
		st := state.Step{
			ID: fmt.Sprintf("s%d", i+1), Title: fmt.Sprintf("step %d", i+1), Kind: "logic",
			Chapter: fmt.Sprintf("chapter %d", i/6+1), Status: state.StatusPending,
		}
		if i < current {
			st.Status = state.StatusDone
		}
		if i%5 == 0 {
			st.Hotspots = []state.Hotspot{{}}
		}
		r.Steps = append(r.Steps, st)
	}
	r.Current = r.Steps[current].ID
	m.review, m.step = r, &r.Steps[current]
	m.height = 16
	m.relist()
	return m
}

func planText(m *model) []string {
	if !m.focusPlan {
		m.focusPlanPanel()
		defer func() { m.focusPlan = false }()
	}
	c := m.planModal(m.width-6, m.height-4)
	var out []string
	for _, line := range c.body {
		out = append(out, ansi.Strip(line))
	}
	return out
}

func planRow(t *testing.T, m *model, want string) int {
	t.Helper()
	for i, line := range planText(m) {
		if strings.Contains(line, want) {
			return i
		}
	}
	t.Fatalf("plan lacks %q:\n%s", want, strings.Join(planText(m), "\n"))
	return -1
}

func ctrlP() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyCtrlP} }

func TestBigPlanKeepsCurrentVisible(t *testing.T) {
	for _, cur := range []int{0, 37, 75} {
		m := bigPlanModel(t, cur)
		plan := strings.Join(planText(m), "\n")
		if !strings.Contains(plan, fmt.Sprintf("▶ s%d ", cur+1)) {
			t.Fatalf("current s%d not shown:\n%s", cur+1, plan)
		}
		if n := strings.Count(plan, "▾"); n != 1 {
			t.Fatalf("want only the current chapter open, %d open:\n%s", n, plan)
		}
	}
	m := bigPlanModel(t, 37)
	if line := planText(m)[planRow(t, m, "CHAPTER 3 ")]; !strings.Contains(line, "▸") ||
		!strings.HasSuffix(strings.TrimSpace(line), "6/6") {
		t.Fatalf("collapsed chapter row: %q", line)
	}
	if line := planText(m)[planRow(t, m, "CHAPTER 7 ")]; !strings.Contains(line, "1/6 ⚑1") {
		t.Fatalf("current chapter row: %q", line)
	}
}

func TestPlanModal(t *testing.T) {
	m := bigPlanModel(t, 37)
	m.Update(key("p"))
	if !m.focusPlan || !strings.Contains(ansi.Strip(m.View()), "┌ plan · 37/76 reviewed") {
		t.Fatalf("p opens the plan modal:\n%s", ansi.Strip(m.View()))
	}
	m.Update(key("p"))
	if m.focusPlan {
		t.Fatal("p again closes it")
	}
}

func TestPlanFoldsChapters(t *testing.T) {
	m := bigPlanModel(t, 37)
	m.Update(ctrlP())
	if !m.focusPlan {
		t.Fatal("ctrl+p must open the plan")
	}
	m.Update(key("h"))
	plan := strings.Join(planText(m), "\n")
	if strings.Contains(plan, "▾") || strings.Contains(plan, "s38 ") {
		t.Fatalf("h must fold the current chapter:\n%s", plan)
	}
	if line := planText(m)[planRow(t, m, "CHAPTER 7 ")]; !strings.HasPrefix(line, "▌") {
		t.Fatalf("cursor must sit on the folded chapter: %q", line)
	}
	m.Update(key("k"))
	m.Update(key("enter"))
	if !strings.Contains(strings.Join(planText(m), "\n"), "s31 ") {
		t.Fatalf("enter must open chapter 6:\n%s", strings.Join(planText(m), "\n"))
	}
	for range 3 {
		m.Update(key("j"))
	}
	m.Update(key("enter"))
	if m.focusPlan || m.step.ID != "s33" || m.review.Current != "s38" {
		t.Fatalf("enter on a step shows it: focus %v step %s current %s",
			m.focusPlan, m.step.ID, m.review.Current)
	}
	m.Update(key("esc"))
	m.Update(ctrlP())
	m.Update(key("esc"))
	if m.focusPlan {
		t.Fatal("esc must close the plan")
	}
}

func TestPlanScrolls(t *testing.T) {
	m := bigPlanModel(t, 0)
	m.planOpen = map[string]bool{}
	for i := range 13 {
		m.planOpen[fmt.Sprintf("chapter %d", i+1)] = true
	}
	m.Update(ctrlP())
	for range 40 {
		m.Update(key("j"))
	}
	plan := strings.Join(planText(m), "\n")
	if !strings.Contains(plan, "s35 ") || strings.Contains(plan, "▶ s1 ") {
		t.Fatalf("j must scroll the plan:\n%s", plan)
	}
	m.Update(key("esc"))
	if plan := strings.Join(planText(m), "\n"); !strings.Contains(plan, "▶ s1 ") {
		t.Fatalf("reopening the plan brings the current step back:\n%s", plan)
	}
	m.Update(ctrlP())
	at := m.planCursor
	m.Update(tea.MouseMsg{X: 2, Y: 5, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	if m.planCursor != at+1 || m.offset != 0 {
		t.Fatalf("wheel over the plan: cursor %d (was %d), code offset %d", m.planCursor, at, m.offset)
	}
}

func TestPlanMouseFoldsChapter(t *testing.T) {
	m := bigPlanModel(t, 37)
	m.Update(ctrlP())
	m.View()
	click := func(want string) {
		m.View()
		y := m.modalY + 1 + planRow(t, m, want)
		m.Update(tea.MouseMsg{X: 4, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	}
	click("CHAPTER 2 ")
	if !strings.Contains(strings.Join(planText(m), "\n"), " s7 ") {
		t.Fatalf("click must open chapter 2:\n%s", strings.Join(planText(m), "\n"))
	}
	click("CHAPTER 2 ")
	if strings.Contains(strings.Join(planText(m), "\n"), " s7 ") {
		t.Fatal("second click must fold chapter 2")
	}
}

func TestFilesScroll(t *testing.T) {
	m, _ := twoFileModel(t)
	for i := range 20 {
		f := fmt.Sprintf("pkg/f%02d.go", i)
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
	if m.chatWidth() != 0 || !strings.Contains(v, "┌ files · s1") || !strings.Contains(v, "c.go") {
		t.Fatalf("narrow: f opens the files modal:\n%s", v)
	}
	m.Update(key("j"))
	m.Update(key("j"))
	m.Update(key("enter"))
	if m.focusFiles || m.current().File != "api/c.go" {
		t.Fatalf("enter in the files modal jumps: focus %v file %s", m.focusFiles, m.current().File)
	}
}
