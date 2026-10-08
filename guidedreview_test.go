package guidedreview_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	guidedreview "github.com/pltanton/guided-review"
)

func TestVersionMatchesThePlugin(t *testing.T) {
	data, err := os.ReadFile(".claude-plugin/plugin.json")
	if err != nil {
		t.Fatal(err)
	}
	var plugin struct{ Version string }
	if err := json.Unmarshal(data, &plugin); err != nil {
		t.Fatal(err)
	}
	if plugin.Version != guidedreview.Version {
		t.Fatalf("plugin.json says %s, the binary %s", plugin.Version, guidedreview.Version)
	}
}

func TestWhatsNew(t *testing.T) {
	got := guidedreview.WhatsNew(guidedreview.Version)
	if got == "" || strings.Contains(got, "## v") {
		t.Fatalf("WhatsNew(%s) = %q, want the section of that version only", guidedreview.Version, got)
	}
	if got := guidedreview.WhatsNew("0.0.0"); got != "" {
		t.Errorf("WhatsNew of an unknown version = %q, want empty", got)
	}
}
