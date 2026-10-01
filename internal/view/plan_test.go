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

func sideText(m *model) []string {
	var out []string
	for _, e := range m.sidebar(max(m.height-len(m.bottomLines()), 1), m.planWidth()) {
		out = append(out, ansi.Strip(e.text))
	}
	return out
}

func sideRow(t *testing.T, m *model, want string) int {
	t.Helper()
	for i, line := range sideText(m) {
		if strings.Contains(line, want) {
			return i
		}
	}
	t.Fatalf("sidebar lacks %q:\n%s", want, strings.Join(sideText(m), "\n"))
	return -1
}

func ctrlP() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyCtrlP} }

func TestBigPlanKeepsCurrentVisible(t *testing.T) {
	for _, cur := range []int{0, 37, 75} {
		m := bigPlanModel(t, cur)
		side := strings.Join(sideText(m), "\n")
		if !strings.Contains(side, fmt.Sprintf("▶ s%d ", cur+1)) {
			t.Fatalf("current s%d not shown:\n%s", cur+1, side)
		}
		if n := strings.Count(side, "▾"); n != 1 {
			t.Fatalf("want only the current chapter open, %d open:\n%s", n, side)
		}
		if !strings.Contains(side, "FILES") {
			t.Fatalf("files section squeezed out:\n%s", side)
		}
	}
	m := bigPlanModel(t, 37)
	if line := sideText(m)[sideRow(t, m, "CHAPTER 3 ")]; !strings.Contains(line, "▸") ||
		!strings.HasSuffix(strings.TrimSpace(line), "6/6") {
		t.Fatalf("collapsed chapter row: %q", line)
	}
	if line := sideText(m)[sideRow(t, m, "CHAPTER 7 ")]; !strings.Contains(line, "1/6 ⚑1") {
		t.Fatalf("current chapter row: %q", line)
	}
	cur, above := sideRow(t, m, "▶ s38 "), sideRow(t, m, "CHAPTER 7 ")
	if below := sideRow(t, m, " s41 "); cur-above < 2 || below-cur < 3 {
		t.Fatalf("no context around the current step:\n%s", strings.Join(sideText(m), "\n"))
	}
}

func TestPlanFoldsChapters(t *testing.T) {
	m := bigPlanModel(t, 37)
	m.Update(ctrlP())
	if !m.focusPlan {
		t.Fatal("ctrl+p must focus the plan")
	}
	m.Update(key("h"))
	side := strings.Join(sideText(m), "\n")
	if strings.Contains(side, "▾") || strings.Contains(side, "s38 ") {
		t.Fatalf("h must fold the current chapter:\n%s", side)
	}
	if line := sideText(m)[sideRow(t, m, "CHAPTER 7 ")]; !strings.HasPrefix(line, "▌") {
		t.Fatalf("cursor must sit on the folded chapter: %q", line)
	}
	m.Update(key("k"))
	m.Update(key("enter"))
	if !strings.Contains(strings.Join(sideText(m), "\n"), "s31 ") {
		t.Fatalf("enter must open chapter 6:\n%s", strings.Join(sideText(m), "\n"))
	}
	for range 3 {
		m.Update(key("j"))
	}
	m.Update(key("enter"))
	if m.focusPlan || m.step.ID != "s33" || m.review.Current != "s38" {
		t.Fatalf("enter on a step previews it: focus %v step %s current %s",
			m.focusPlan, m.step.ID, m.review.Current)
	}
	m.Update(key("esc"))
	m.Update(ctrlP())
	m.Update(key("esc"))
	if m.focusPlan {
		t.Fatal("esc must leave the plan")
	}
}

func TestPlanScrolls(t *testing.T) {
	m := bigPlanModel(t, 0)
	m.planOpen = map[string]bool{}
	for i := range 13 {
		m.planOpen[fmt.Sprintf("chapter %d", i+1)] = true
	}
	if side := strings.Join(sideText(m), "\n"); !strings.Contains(side, "↓") ||
		strings.Contains(side, "↑") {
		t.Fatalf("plan header must count rows below only:\n%s", side)
	}
	m.Update(ctrlP())
	for range 40 {
		m.Update(key("j"))
	}
	side := strings.Join(sideText(m), "\n")
	if !strings.Contains(side, "↑") || !strings.Contains(side, "s35 ") ||
		strings.Contains(side, "▶ s1 ") {
		t.Fatalf("j must scroll the plan:\n%s", side)
	}
	m.Update(key("esc"))
	if side := strings.Join(sideText(m), "\n"); !strings.Contains(side, "▶ s1 ") {
		t.Fatalf("leaving the plan brings the current step back:\n%s", side)
	}
	m.Update(tea.MouseMsg{X: 2, Y: 1, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	if side := strings.Join(sideText(m), "\n"); m.planTop != wheelStep || m.offset != 0 ||
		!strings.Contains(side, "↑3 ") {
		t.Fatalf("wheel over the plan: top %d, code offset %d:\n%s", m.planTop, m.offset, side)
	}
}

func TestPlanMouseFoldsChapter(t *testing.T) {
	m := bigPlanModel(t, 37)
	y := sideRow(t, m, "CHAPTER 2 ")
	click := tea.MouseMsg{X: 2, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
	m.Update(click)
	if !strings.Contains(strings.Join(sideText(m), "\n"), " s7 ") {
		t.Fatalf("click must open chapter 2:\n%s", strings.Join(sideText(m), "\n"))
	}
	m.Update(tea.MouseMsg{X: 2, Y: sideRow(t, m, "CHAPTER 2 "), Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress})
	if strings.Contains(strings.Join(sideText(m), "\n"), " s7 ") {
		t.Fatal("second click must fold chapter 2")
	}
}

func TestSidebarSharesHeight(t *testing.T) {
	m := bigPlanModel(t, 37)
	m.planOpen = map[string]bool{"chapter 6": true, "chapter 8": true}
	h := max(m.height-len(m.bottomLines()), 1)
	if got := sideRow(t, m, "FILES"); got != h-2 {
		t.Fatalf("one file: FILES at row %d, want %d", got, h-2)
	}

	m, _ = twoFileModel(t)
	for i := range 20 {
		f := fmt.Sprintf("pkg/f%02d.go", i)
		m.rows = append(m.rows, Row{Kind: RowFile, File: f, Text: f},
			Row{Kind: RowAdded, File: f, Line: 1, Text: "x", HunkStart: true})
	}
	m.height = 16
	m.relist()
	m.Update(key("f"))
	for range 18 {
		m.Update(key("j"))
	}
	side := strings.Join(sideText(m), "\n")
	if !strings.Contains(side, "f15.go") || !strings.Contains(side, "↑") {
		t.Fatalf("files must scroll to the cursor:\n%s", side)
	}
	m.Update(key("enter"))
	if m.current().File != "pkg/f15.go" {
		t.Fatalf("enter in files: %+v", m.current())
	}
}
