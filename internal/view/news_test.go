package view

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	guidedreview "github.com/pltanton/guided-review"
)

func TestWhatsNewOnce(t *testing.T) {
	m, _ := newTestModel(t)
	m.newsFile = filepath.Join(t.TempDir(), "seen-version")
	v := ansi.Strip(m.View())
	if !m.newsOpen || !strings.Contains(v, "┌ what's new in v"+guidedreview.Version) ||
		!strings.Contains(v, "Threads in the code") {
		t.Fatalf("the first view after an update shows what's new:\n%s", v)
	}
	m.Update(key("x"))
	if m.newsOpen {
		t.Fatal("any key closes it")
	}
	if seen, err := os.ReadFile(m.newsFile); err != nil ||
		strings.TrimSpace(string(seen)) != guidedreview.Version {
		t.Fatalf("closing it remembers the version: %q %v", seen, err)
	}

	again, _ := newTestModel(t)
	again.newsFile = m.newsFile
	again.View()
	if again.newsOpen {
		t.Fatal("a version already seen shows nothing")
	}
	again.Update(key(":"))
	typeText(again, "changelog")
	again.Update(key("enter"))
	if v := ansi.Strip(again.View()); !again.newsOpen || !strings.Contains(v, "┌ changelog") ||
		!strings.Contains(v, "v0.1.0") {
		t.Fatalf(":changelog shows every version:\n%s", v)
	}
}
