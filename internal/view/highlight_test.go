package view

import (
	"reflect"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestHighlightKeepsText(t *testing.T) {
	src := "package api\n\n/* multi\nline */\nfunc F() int {\n    return 1\n}\n"
	got := Highlight("a.go", src)
	var plain []string
	for _, l := range got {
		plain = append(plain, ansi.Strip(l))
	}
	want := []string{"package api", "", "/* multi", "line */", "func F() int {", "    return 1", "}"}
	if !reflect.DeepEqual(plain, want) {
		t.Fatalf("got %q, want %q", plain, want)
	}
	if got := Highlight("x.unknown-ext", "a\nb\n"); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("fallback: %q", got)
	}
	if got := Highlight("a.go", ""); got != nil {
		t.Fatalf("empty: %q", got)
	}
}
