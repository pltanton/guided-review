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

const longAnswer = "the fee is rounded before the currency is converted, so a small transfer " +
	"loses a cent\nsecond thought"

func chatModel(t *testing.T) (*model, *string) {
	t.Helper()
	m, _ := newTestModel(t)
	m.width = 150
	t0 := time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)
	m.review.Messages = []state.Message{
		{Time: t0, Step: "s1", Text: longAnswer},
		{Time: t0.Add(time.Second), Step: "s1", Text: "short one"},
	}
	var copied string
	m.clip = func(s string) error { copied = s; return nil }
	return m, &copied
}

func TestChatSelectAndCopyKeys(t *testing.T) {
	m, copied := chatModel(t)
	m.Update(key("t"))
	if v := ansi.Strip(m.View()); !m.chatFocus || !strings.Contains(v, "j/k move · v select") {
		t.Fatalf("t focuses the chat and says so:\n%s", v)
	}
	m.Update(key("y"))
	if *copied != "short one" || m.status != "copied 1 line" {
		t.Fatalf("y copies the message under the cursor: %q %q", *copied, m.status)
	}
	m.Update(key("k"))
	m.Update(key("y"))
	if m.status != "nothing to copy here" {
		t.Fatalf("the gap between messages has nothing to copy: %q", m.status)
	}
	m.Update(key("k"))
	m.Update(key("y"))
	if *copied != longAnswer {
		t.Fatalf("y copies the whole wrapped message as written: %q", *copied)
	}
	m.Update(key("v"))
	m.Update(key("k"))
	m.Update(key("y"))
	if *copied != longAnswer || m.chatVisual {
		t.Fatalf("a selection copies whole source lines without gutters: %q", *copied)
	}
	m.Update(key("v"))
	m.Update(key("esc"))
	if !m.chatFocus || m.chatVisual {
		t.Fatal("esc first drops the selection")
	}
	m.Update(key("esc"))
	if m.chatFocus {
		t.Fatal("esc then returns to the code")
	}
	m.Update(key("j"))
	if m.cursor == 0 {
		t.Fatal("j moves the code cursor again")
	}
}

func TestChatMouseDragCopies(t *testing.T) {
	for _, width := range []int{150, 120} {
		m, copied := chatModel(t)
		m.width, m.chatFocus = width, true
		lines := strings.Split(ansi.Strip(m.View()), "\n")
		at := func(s string) int {
			return slices.IndexFunc(lines, func(l string) bool { return strings.Contains(l, s) })
		}
		from, to := at("the fee is"), at("short one")
		if from < 0 || to <= from {
			t.Fatalf("width %d: chat rows not found:\n%s", width, strings.Join(lines, "\n"))
		}
		x := m.width - 10
		press := tea.MouseMsg{X: x, Y: from, Button: tea.MouseButtonLeft,
			Action: tea.MouseActionPress}
		m.Update(press)
		m.Update(tea.MouseMsg{X: x, Y: to, Action: tea.MouseActionMotion})
		if !m.chatVisual {
			t.Fatalf("width %d: dragging over chat rows selects them", width)
		}
		m.Update(tea.MouseMsg{X: x, Y: to, Action: tea.MouseActionRelease})
		if *copied != longAnswer+"\nshort one" || m.status != "copied 3 lines" || m.chatFocus {
			t.Fatalf("width %d: release copies the dragged lines: %q %q", width, *copied, m.status)
		}

		*copied = ""
		m.Update(press)
		m.Update(tea.MouseMsg{X: x, Y: from, Action: tea.MouseActionRelease})
		if *copied != "" || m.composing || m.chatFocus {
			t.Fatalf("width %d: a click without a drag copies nothing", width)
		}
	}
}
