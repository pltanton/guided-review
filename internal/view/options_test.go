package view

import (
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/pltanton/guided-review/internal/state"
)

func TestAnswerPillsFitTheWidth(t *testing.T) {
	m, sent := newTestModel(t)
	t0 := time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)
	options := []string{"да, всё верно", "нет, поправлю", "не знаю, посмотри сам"}
	m.review.Messages = []state.Message{{Time: t0, Step: "s1", Text: "?", Options: options}}

	lines := strings.Split(ansi.Strip(m.View()), "\n")
	i := slices.IndexFunc(lines, func(l string) bool { return strings.Contains(l, "alt+1 да") })
	if i < 0 || !strings.Contains(lines[i], "alt+3 не знаю") ||
		!strings.HasPrefix(lines[i+1], "c own answer") {
		t.Fatalf("wide: pills in one row, the hint on its own line:\n%s", strings.Join(lines, "\n"))
	}

	m.width = 150
	lines = strings.Split(ansi.Strip(m.View()), "\n")
	var rows []int
	for k, o := range options {
		label := "alt+" + string(rune('1'+k)) + " " + o
		j := slices.IndexFunc(lines, func(l string) bool { return strings.Contains(l, label) })
		if j < 0 {
			t.Fatalf("side chat: pill %q is whole:\n%s", label, strings.Join(lines, "\n"))
		}
		rows = append(rows, j)
	}
	if rows[1] != rows[0]+1 || rows[2] != rows[1]+1 ||
		!strings.Contains(lines[rows[2]+1], "c own answer") {
		t.Fatalf("side chat: one pill per line, then the hint: rows %v\n%s", rows,
			strings.Join(lines, "\n"))
	}

	x := strings.Index(lines[rows[1]], "alt+2")
	x = len([]rune(lines[rows[1]][:x])) + 1
	m.Update(tea.MouseMsg{X: x, Y: rows[1], Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress})
	if len(*sent) != 1 || (*sent)[0].Text != "нет, поправлю" {
		t.Fatalf("a click on the second pill's line answers with it: %+v", *sent)
	}
}
