package view

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/pltanton/guided-review/internal/config"
)

const longCode = "total := fee(amount, rate, " + "padding, " + "padding, padding, padding, " +
	"padding, padding, padding, padding, padding, padding, padding, padding, roundUp)"

func longLineModel(t *testing.T) *model {
	t.Helper()
	m, _ := newTestModel(t)
	m.rows[2].Text, m.rows[2].Plain = longCode, longCode
	m.relist()
	return m
}

func screenRows(m *model) []string {
	return strings.Split(ansi.Strip(m.View()), "\n")
}

func rowsWith(rows []string, s string) int {
	for i, r := range rows {
		if strings.Contains(r, s) {
			return i
		}
	}
	return -1
}

func TestLongLinesWrap(t *testing.T) {
	m := longLineModel(t)
	rows := screenRows(m)
	head, tail := rowsWith(rows, "+   2 │ total := fee("), rowsWith(rows, "roundUp)")
	if head < 0 || tail <= head {
		t.Fatalf("a long line wraps and its tail is on screen:\n%s", strings.Join(rows, "\n"))
	}
	pw := m.planWidth()
	for _, r := range rows[head+1 : tail+1] {
		if code := []rune(r)[pw:]; !strings.HasPrefix(string(code), "      │ ") {
			t.Fatalf("a continuation keeps the gutter without a number: %q", string(code))
		}
	}
	if !strings.Contains(rows[tail+1], "NOTE  why x") {
		t.Fatalf("the note follows the wrapped line: %q", rows[tail+1])
	}
	if m.unseen() != 0 {
		t.Fatalf("a wrapped line on screen is seen, unseen %d", m.unseen())
	}

	m.cursor = 2
	m.Update(key("j"))
	if m.cursor != 3 {
		t.Fatalf("j moves by logical lines: cursor %d", m.cursor)
	}
	m.Update(tea.MouseMsg{X: pw + codePrefix + 2, Y: tail, Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress})
	if avail := codeAvail(m.mainWidth()); m.cursor != 2 || m.col != (tail-head)*avail+2 {
		t.Fatalf("a click on a continuation picks the line and its column: cursor %d col %d",
			m.cursor, m.col)
	}

	m.splitView, m.width = true, 200
	m.relist()
	rows = screenRows(m)
	if rowsWith(rows, "roundUp)") <= rowsWith(rows, "total := fee(") {
		t.Fatalf("split view wraps each side in its column:\n%s", strings.Join(rows, "\n"))
	}
}

func TestNoWrapScrollsSideways(t *testing.T) {
	m := longLineModel(t)
	m.applyConfig(config.Config{View: config.View{NoWrap: true}})
	m.seen = nil
	rows := screenRows(m)
	line := rows[rowsWith(rows, "total := fee(")]
	if strings.Contains(line, "roundUp)") || !strings.HasSuffix(strings.TrimRight(line, " "), "›") {
		t.Fatalf("without wrap a long line is cut and marked with ›: %q", line)
	}
	if m.unseen() != 1 {
		t.Fatalf("a cut line is not seen yet: unseen %d", m.unseen())
	}
	for range 20 {
		m.Update(key("l"))
	}
	rows = screenRows(m)
	if rowsWith(rows, "roundUp)") < 0 || m.unseen() != 0 {
		t.Fatalf("l scrolls to the tail and that sees it: unseen %d\n%s", m.unseen(),
			strings.Join(rows, "\n"))
	}
	if m.hscroll != m.maxHScroll() {
		t.Fatalf("l stops at the longest tail: %d of %d", m.hscroll, m.maxHScroll())
	}
	m.Update(key("h"))
	if m.hscroll != m.maxHScroll()-hscrollStep {
		t.Fatalf("h scrolls back: %d", m.hscroll)
	}

	m.execCommand("set wrap")
	if m.nowrap || m.hscroll != 0 {
		t.Fatal(":set wrap turns wrapping on")
	}
	m.Update(key("l"))
	if m.hscroll != 0 || !strings.Contains(m.status, "wrap") {
		t.Fatalf("with wrap l only explains: %q", m.status)
	}
	m.execCommand("set nowrap")
	if !m.nowrap {
		t.Fatal(":set nowrap turns wrapping off")
	}
	m.Update(key("W"))
	if m.nowrap {
		t.Fatal("W toggles wrapping")
	}
}

func TestWrapKeepsIntralineEmphasis(t *testing.T) {
	m := &model{}
	text := strings.Repeat("a", 30) + strings.Repeat("b", 30)
	c := Cell{Line: 7, Kind: RowAdded, Text: text, Emph: [][2]int{{25, 45}}}
	rows := m.codeRows(c, false, codePrefix+20)
	if len(rows) != 3 {
		t.Fatalf("60 cells in a 20-cell column make 3 rows, got %d", len(rows))
	}
	if !strings.Contains(rows[1], "\x1b[1m") || !strings.Contains(rows[2], "\x1b[22m") {
		t.Fatalf("an emphasis across the wrap stays on the next row: %q", rows)
	}
	got := ansi.Strip(strings.Join(rows, ""))
	if strings.Count(got, "a") != 30 || strings.Count(got, "b") != 30 {
		t.Fatalf("wrapping loses no text: %q", got)
	}
}

func TestWrapKeepsIndentAndBreaksAfterSpace(t *testing.T) {
	spans, indent := wrapSpans("    log.Printf(\"one two three four\", a, b)", 20)
	if indent != 4 {
		t.Fatalf("indent %d, want 4", indent)
	}
	plain := "    log.Printf(\"one two three four\", a, b)"
	var got []string
	for _, s := range spans {
		got = append(got, plain[s[0]:s[1]])
	}
	want := []string{"    log.Printf(\"one ", "two three ", "four\", a, b)"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("rows %q, want %q", got, want)
	}
	for i, s := range spans[1:] {
		if s[1]-s[0] > 20-indent {
			t.Fatalf("row %d is %d wide, over %d", i+1, s[1]-s[0], 20-indent)
		}
	}
}
