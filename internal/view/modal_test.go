package view

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestModalBox(t *testing.T) {
	body := []string{"a", "b", "c", "d", "e", "f", "g"}
	box := modalBox(40, 8, "plan", "esc close", body)
	if len(box) != 8 {
		t.Fatalf("len(box) = %d, want 8", len(box))
	}
	for i, l := range box {
		if w := ansi.StringWidth(l); w != 40 {
			t.Errorf("line %d width = %d, want 40: %q", i, w, ansi.Strip(l))
		}
	}
	first, last := ansi.Strip(box[0]), ansi.Strip(box[7])
	if !strings.HasPrefix(first, "┌ plan ") || !strings.HasSuffix(first, "┐") {
		t.Errorf("head = %q", first)
	}
	if !strings.HasPrefix(last, "└") || !strings.HasSuffix(last, "esc close┘") {
		t.Errorf("foot = %q", last)
	}
	if got := ansi.Strip(box[1]); !strings.HasPrefix(got, "│ a") || !strings.HasSuffix(got, "│") {
		t.Errorf("body = %q", got)
	}
	if got := ansi.Strip(box[6]); !strings.HasPrefix(got, "│ f") {
		t.Errorf("last body line = %q, want f (g cut)", got)
	}
}

func TestOverlayAt(t *testing.T) {
	base := []string{"0123456789", "abcdefghij", "ABCDEFGHIJ"}
	overlayAt(base, []string{"xx", "yy"}, 1, 3)
	want := []string{"0123456789", "abcxxfghij", "ABCyyFGHIJ"}
	for i := range want {
		if got := ansi.Strip(base[i]); got != want[i] {
			t.Errorf("line %d = %q, want %q", i, got, want[i])
		}
	}
}
