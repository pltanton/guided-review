package view

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

type palette struct {
	chroma                       string
	bg, fg, muted, accent, agent string
	you, ok, warn, bad, blue     string
}

const defaultTheme = "default"

var palettes = map[string]palette{
	"catppuccin-mocha": {"catppuccin-mocha", "#1e1e2e", "#cdd6f4", "#7f849c", "#cba6f7",
		"#89dceb", "#f5c2e7", "#a6e3a1", "#f9e2af", "#f38ba8", "#89b4fa"},
	"catppuccin-latte": {"catppuccin-latte", "#eff1f5", "#4c4f69", "#8c8fa1", "#8839ef",
		"#04a5e5", "#ea76cb", "#40a02b", "#df8e1d", "#d20f39", "#1e66f5"},
	"gruvbox-dark": {"gruvbox", "#282828", "#ebdbb2", "#928374", "#d3869b",
		"#8ec07c", "#fe8019", "#b8bb26", "#fabd2f", "#fb4934", "#83a598"},
	"gruvbox-light": {"gruvbox-light", "#fbf1c7", "#3c3836", "#928374", "#8f3f71",
		"#427b58", "#af3a03", "#79740e", "#b57614", "#9d0006", "#076678"},
	"nord": {"nord", "#2e3440", "#eceff4", "#7b88a1", "#b48ead",
		"#88c0d0", "#d08770", "#a3be8c", "#ebcb8b", "#bf616a", "#81a1c1"},
	"dracula": {"dracula", "#282a36", "#f8f8f2", "#6272a4", "#bd93f9",
		"#8be9fd", "#ff79c6", "#50fa7b", "#f1fa8c", "#ff5555", "#8be9fd"},
	"tokyonight": {"tokyonight-night", "#1a1b26", "#c0caf5", "#565f89", "#bb9af7",
		"#7dcfff", "#ff9e64", "#9ece6a", "#e0af68", "#f7768e", "#7aa2f7"},
	"one-dark": {"onedark", "#282c34", "#abb2bf", "#5c6370", "#c678dd",
		"#56b6c2", "#d19a66", "#98c379", "#e5c07b", "#e06c75", "#61afef"},
	"solarized-dark": {"solarized-dark", "#002b36", "#93a1a1", "#586e75", "#6c71c4",
		"#2aa198", "#d33682", "#859900", "#b58900", "#dc322f", "#268bd2"},
	"solarized-light": {"solarized-light", "#fdf6e3", "#586e75", "#93a1a1", "#6c71c4",
		"#2aa198", "#d33682", "#859900", "#b58900", "#dc322f", "#268bd2"},
	"github-dark": {"github-dark", "#0d1117", "#c9d1d9", "#8b949e", "#bc8cff",
		"#39c5cf", "#f778ba", "#3fb950", "#d29922", "#f85149", "#58a6ff"},
	"github-light": {"github", "#ffffff", "#1f2328", "#656d76", "#8250df",
		"#1b7c83", "#bf3989", "#1a7f37", "#9a6700", "#cf222e", "#0969da"},
}

func Themes() []string {
	names := []string{defaultTheme}
	for name := range palettes {
		names = append(names, name)
	}
	slices.Sort(names[1:])
	return names
}

type toneSet struct {
	accent, agent, you, ok, warn, bad, blue, text, noteText, muted, faint tone
	surface, cursor, sel, file, addBg, addLine, delLine, delBg, ink       tone
	noteBg                                                                map[string]tone
	chroma                                                                string
}

var defaultTones = toneSet{
	accent: accentTone, agent: agentTone, you: youTone, ok: okTone, warn: warnTone,
	bad: badTone, blue: blueTone, text: textTone, noteText: noteTextTone, muted: mutedTone,
	faint: faintTone, surface: surfaceTone, cursor: cursorTone, sel: selectTone,
	file: fileTone, addBg: addBgTone, addLine: addLineTone, delLine: delLineTone,
	delBg: delBgTone, ink: inkTone, chroma: "monokai",
	noteBg: map[string]tone{
		"note": {"#E8F6F8", "#162529"}, "spec": {"#FBEDED", "#2A1A1D"},
		"hotspot": {"#FBF4E4", "#292316"}, "comment": {"#FAEBF3", "#291827"},
		"mr": {"#EAF0FB", "#172033"}, "pending": {"#FBF4E4", "#292316"},
	},
}

var themeName = defaultTheme

func applyTheme(name string) error {
	set := defaultTones
	if name != defaultTheme {
		p, ok := palettes[name]
		if !ok {
			return fmt.Errorf("unknown theme %q, one of %s", name, strings.Join(Themes(), ", "))
		}
		set = p.tones()
	}
	accentTone, agentTone, youTone, okTone, warnTone = set.accent, set.agent, set.you, set.ok, set.warn
	badTone, blueTone, textTone, noteTextTone = set.bad, set.blue, set.text, set.noteText
	mutedTone, faintTone, surfaceTone, cursorTone = set.muted, set.faint, set.surface, set.cursor
	selectTone, fileTone, addBgTone, addLineTone = set.sel, set.file, set.addBg, set.addLine
	delLineTone, delBgTone, inkTone = set.delLine, set.delBg, set.ink
	themeName, styleName = name, set.chroma
	restyle(set.noteBg)
	return nil
}

func (p palette) tones() toneSet {
	one := func(hex string) tone { return tone{hex, hex} }
	mix := func(hex string, t float64) tone { return one(blend(hex, p.bg, t)) }
	return toneSet{
		accent: one(p.accent), agent: one(p.agent), you: one(p.you), ok: one(p.ok),
		warn: one(p.warn), bad: one(p.bad), blue: one(p.blue), text: one(p.fg),
		noteText: mix(p.fg, 0.82), muted: one(p.muted), faint: mix(p.fg, 0.32),
		surface: mix(p.fg, 0.08), cursor: mix(p.fg, 0.14), sel: mix(p.accent, 0.3),
		file: mix(p.accent, 0.14), addBg: mix(p.ok, 0.34), addLine: mix(p.ok, 0.1),
		delLine: mix(p.bad, 0.1), delBg: mix(p.bad, 0.34), ink: one(p.bg), chroma: p.chroma,
		noteBg: map[string]tone{
			"note": mix(p.agent, 0.12), "spec": mix(p.bad, 0.12), "hotspot": mix(p.warn, 0.12),
			"comment": mix(p.you, 0.12), "mr": mix(p.blue, 0.12), "pending": mix(p.warn, 0.12),
		},
	}
}

func blend(fg, bg string, t float64) string {
	a, b := rgb(fg), rgb(bg)
	var out [3]int
	for i := range out {
		out[i] = int(float64(a[i])*t + float64(b[i])*(1-t) + 0.5)
	}
	return fmt.Sprintf("#%02x%02x%02x", out[0], out[1], out[2])
}

func rgb(hex string) [3]int {
	v, err := strconv.ParseUint(strings.TrimPrefix(hex, "#"), 16, 32)
	if err != nil {
		return [3]int{}
	}
	return [3]int{int(v >> 16), int(v >> 8 & 0xff), int(v & 0xff)}
}
