package view

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type tone struct{ light, dark string }

func (t tone) color() lipgloss.AdaptiveColor {
	return lipgloss.AdaptiveColor{Light: t.light, Dark: t.dark}
}

func (t tone) fg() lipgloss.Style { return lipgloss.NewStyle().Foreground(t.color()) }

var (
	accentTone  = tone{"#6D28D9", "#A78BFA"}
	agentTone   = tone{"#0E7490", "#67E8F9"}
	youTone     = tone{"#BE185D", "#F9A8D4"}
	okTone      = tone{"#047857", "#6EE7B7"}
	warnTone    = tone{"#B45309", "#FCD34D"}
	badTone     = tone{"#B91C1C", "#FCA5A5"}
	blueTone    = tone{"#1D4ED8", "#93C5FD"}
	textTone    = tone{"#1F2937", "#E5E7EB"}
	mutedTone   = tone{"#6B7280", "#7C8394"}
	faintTone   = tone{"#9CA3AF", "#4B5263"}
	surfaceTone = tone{"#EEF0F6", "#232737"}
	cursorTone  = tone{"#E3E7F2", "#2B3044"}
	selectTone  = tone{"#DAD5FB", "#3A3360"}
	fileTone    = tone{"#ECE9FD", "#29243F"}
	addBgTone   = tone{"#BBF7D0", "#14532D"}
	addLineTone = tone{"#F0FAF3", "#18261D"}
	delLineTone = tone{"#FCF1F1", "#2A1B1D"}
	delBgTone   = tone{"#FECACA", "#7F1D1D"}
	inkTone     = tone{"#FFFFFF", "#111827"}
)

// paint puts a background under an already styled line. Styled spans end with a full
// SGR reset, so the background is re-applied after each one.
func paint(line string, bg tone) string {
	seq := bgSeq(bg)
	return seq + strings.ReplaceAll(line, "\x1b[0m", "\x1b[0m"+seq) + "\x1b[0m"
}

func bgSeq(t tone) string {
	hex := t.dark
	if !lipgloss.HasDarkBackground() {
		hex = t.light
	}
	v, err := strconv.ParseUint(strings.TrimPrefix(hex, "#"), 16, 32)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", v>>16, v>>8&0xff, v&0xff)
}
