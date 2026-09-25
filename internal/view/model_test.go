package view

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func key(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestModelNavigation(t *testing.T) {
	m := &model{
		rows: []Row{
			{Kind: RowFile, File: "a.go"},
			{Kind: RowCode, File: "a.go", Line: 1},
			{Kind: RowAdded, File: "a.go", Line: 2, HunkStart: true},
			{Kind: RowCode, File: "a.go", Line: 3},
			{Kind: RowAdded, File: "a.go", Line: 9, HunkStart: true},
		},
		width: 80, height: 20,
	}
	steps := []struct {
		key  string
		want int
	}{{"]", 2}, {"]", 4}, {"]", 4}, {"[", 2}, {"j", 3}, {"k", 2}, {"G", 4}, {"g", 0}}
	for _, s := range steps {
		m.Update(key(s.key))
		if m.cursor != s.want {
			t.Fatalf("after %q cursor = %d, want %d", s.key, m.cursor, s.want)
		}
	}
	if _, cmd := m.Update(key("q")); cmd == nil {
		t.Fatal("q must quit")
	}
}

func TestFirstFocus(t *testing.T) {
	tests := []struct {
		name string
		rows []Row
		want int
	}{
		{"hunk start", []Row{{Kind: RowFile}, {Kind: RowCode, Line: 1}, {Kind: RowAdded, Line: 2, HunkStart: true}}, 2},
		{"hunk outside window", []Row{{Kind: RowFile}, {Kind: RowAdded, Line: 76}}, 1},
		{"empty", nil, 0},
	}
	for _, tt := range tests {
		if got := firstFocus(tt.rows); got != tt.want {
			t.Errorf("%s: firstFocus = %d, want %d", tt.name, got, tt.want)
		}
	}
}
