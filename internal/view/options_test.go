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
	i := slices.IndexFunc(lines, func(l string) bool { return strings.Contains(l, "1 да") })
	if i < 0 || !strings.Contains(lines[i], "3 не знаю") ||
		!strings.HasPrefix(lines[i+1], "c own answer") {
		t.Fatalf("wide: pills in one row, the hint on its own line:\n%s", strings.Join(lines, "\n"))
	}

	x := strings.Index(lines[i], "2 нет")
	x = len([]rune(lines[i][:x])) + 1
	m.Update(tea.MouseMsg{X: x, Y: i, Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress})
	if len(*sent) != 1 || (*sent)[0].Text != "нет, поправлю" {
		t.Fatalf("a click on the second pill answers with it: %+v", *sent)
	}
}
