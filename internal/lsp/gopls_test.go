package lsp

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestGopls(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not installed")
	}
	dir := t.TempDir()
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.com/m\n\ngo 1.22\n")
	write("a.go", "package m\n\nfunc Use() int {\n\treturn Transfer(1, 2)\n}\n")
	write(
		"b.go",
		"package m\n\n// Transfer adds.\nfunc Transfer(a, b int) int {\n\treturn a + b\n}\n",
	)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	c, err := Start(ctx, []string{"gopls"}, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Shutdown(ctx) }()
	a := filepath.Join(dir, "a.go")
	data, _ := os.ReadFile(a)
	if err := c.DidOpen(a, "go", string(data)); err != nil {
		t.Fatal(err)
	}
	defs, err := c.Definition(ctx, a, 3, 8)
	if err != nil || len(defs) != 1 || filepath.Base(defs[0].Path) != "b.go" || defs[0].Line != 3 {
		t.Fatalf("Definition = %+v, %v", defs, err)
	}
	b := filepath.Join(dir, "b.go")
	data, _ = os.ReadFile(b)
	_ = c.DidOpen(b, "go", string(data))
	refs, err := c.References(ctx, b, 3, 6)
	if err != nil || len(refs) != 1 || filepath.Base(refs[0].Path) != "a.go" || refs[0].Line != 3 {
		t.Fatalf("References = %+v, %v", refs, err)
	}
	hover, err := c.Hover(ctx, a, 3, 8)
	if err != nil || hover == "" {
		t.Fatalf("Hover = %q, %v", hover, err)
	}
	t.Logf("hover: %q", hover)
}
