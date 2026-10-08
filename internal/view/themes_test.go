package view

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestEveryThemeApplies(t *testing.T) {
	t.Cleanup(func() { _ = applyTheme(defaultTheme) })
	for _, name := range Themes() {
		if err := applyTheme(name); err != nil {
			t.Fatalf("applyTheme(%q): %v", name, err)
		}
		if p, ok := palettes[name]; ok && (accentTone.dark != p.accent || styleName != p.chroma) {
			t.Errorf("%s: accent %s style %s, want %s %s", name, accentTone.dark, styleName,
				p.accent, p.chroma)
		}
	}
	if err := applyTheme("nope"); err == nil || !strings.Contains(err.Error(), "catppuccin-mocha") {
		t.Fatalf("an unknown theme names the known ones: %v", err)
	}
	if err := applyTheme(defaultTheme); err != nil || accentTone != defaultTones.accent ||
		styleName != "monokai" {
		t.Fatalf("default restores the built-in colours: %v %v %s", err, accentTone, styleName)
	}
}

func TestThemeCard(t *testing.T) {
	t.Cleanup(func() { _ = applyTheme(defaultTheme) })
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m, _ := newTestModel(t)
	m.Update(key(":"))
	typeText(m, "theme")
	m.Update(key("enter"))
	if v := ansi.Strip(m.View()); !strings.Contains(v, "┌ theme") ||
		!strings.Contains(v, "default  (was)") || !strings.Contains(v, "nord") {
		t.Fatalf(":theme opens the theme card:\n%s", v)
	}
	m.Update(key("j"))
	if themeName != Themes()[1] {
		t.Fatalf("j tries the next theme at once: %s", themeName)
	}
	m.Update(key("esc"))
	if themeName != defaultTheme {
		t.Fatalf("esc goes back to the theme it had: %s", themeName)
	}
	m.Update(key(":"))
	typeText(m, "theme")
	m.Update(key("enter"))
	m.Update(key("j"))
	m.Update(key("enter"))
	data, err := os.ReadFile(filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "guided-review",
		"config.yaml"))
	if err != nil || !strings.Contains(string(data), "theme: "+Themes()[1]) {
		t.Fatalf("enter saves the theme: %q %v", data, err)
	}
}
