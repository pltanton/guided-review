package view

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/pltanton/guided-review/internal/state"
)

// reading is true while the agent works: the block shows what it has read so far.
func (m *model) reading() bool {
	return m.review != nil && !m.agentWaiting && !m.agentIdle
}

// flagArt is the logo flag in half-block pixels: a red banner with a notched tail on a wooden pole.
func flagArt() []string {
	px := func(top, bottom string) string {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(top)).Background(lipgloss.Color(bottom)).Render("▀")
	}
	wood, red, bright, none := "#8c6239", "#e64553", "#f06a76", ""
	cell := func(top, bottom string) string {
		switch {
		case top == none && bottom == none:
			return " "
		case bottom == none:
			return lipgloss.NewStyle().Foreground(lipgloss.Color(top)).Render("▀")
		case top == none:
			return lipgloss.NewStyle().Foreground(lipgloss.Color(bottom)).Render("▄")
		}
		return px(top, bottom)
	}
	rows := [][2][]string{
		{{wood, bright, bright, bright, bright, bright, bright}, {wood, red, red, red, red, red, none}},
		{{wood, red, red, red, red, red, none}, {wood, red, red, red, red, red, red}},
		{{wood, none, none, none, none, none, none}, {wood, none, none, none, none, none, none}},
	}
	out := make([]string, len(rows))
	for i, r := range rows {
		var b strings.Builder
		for x := range r[0] {
			b.WriteString(cell(r[0][x], r[1][x]))
		}
		out[i] = b.String()
	}
	return out
}

// readingBlock is the side-column block shown while the agent reads: the flag, one square per
// core file (lit once read, the next one blinking) and the agent's progress line.
func (m *model) readingBlock(w int) []string {
	var squares []string
	blink := m.frame/3%2 == 0
	first := true
	for _, f := range m.review.FileOrder() {
		if f.Tier != state.TierCore {
			continue
		}
		switch {
		case f.Read:
			squares = append(squares, addStyle.Render("■"))
		case first && blink:
			squares = append(squares, hotStyle.Render("■"))
			first = false
		case first:
			squares = append(squares, faintTone.fg().Render("■"))
			first = false
		default:
			squares = append(squares, faintTone.fg().Render("■"))
		}
	}
	read, core := 0, len(squares)
	for _, f := range m.review.Files {
		if f.Tier == state.TierCore && f.Read {
			read++
		}
	}
	perRow := max(w-10, 4)
	var rows []string
	for len(squares) > 0 {
		n := min(perRow, len(squares))
		rows = append(rows, strings.Join(squares[:n], ""))
		squares = squares[n:]
	}
	art := flagArt()
	label := hotStyle.Render("agent at work") + dimStyle.Render(fmt.Sprintf(" · read %d/%d", read, core))
	lines := []string{" " + art[0] + "  " + label}
	for i := 1; i < len(art); i++ {
		right := ""
		if i-1 < len(rows) {
			right = rows[i-1]
		}
		lines = append(lines, " "+art[i]+"  "+right)
	}
	for i := len(art) - 1; i < len(rows); i++ {
		lines = append(lines, "          "+rows[i])
	}
	text := "agent working"
	if m.review.Progress != nil {
		text = m.review.Progress.Text
	}
	lines = append(lines, " "+dimStyle.Render(fit(text, w-1)), "")
	return lines
}
