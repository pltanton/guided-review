package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"
)

type fakeServer struct {
	in  *bufio.Reader
	out io.Writer
	t   *testing.T
}

func (s *fakeServer) serve() {
	for {
		body, err := readMessage(s.in)
		if err != nil {
			return
		}
		var msg struct {
			ID     *json.RawMessage `json:"id"`
			Method string           `json:"method"`
		}
		_ = json.Unmarshal(body, &msg)
		if msg.ID == nil {
			continue
		}
		var result any
		switch msg.Method {
		case "initialize":
			result = map[string]any{"capabilities": map[string]any{}}
			_ = writeMessage(
				s.out,
				map[string]any{
					"jsonrpc": "2.0",
					"id":      900,
					"method":  "workspace/configuration",
					"params":  map[string]any{"items": []any{map[string]any{}}},
				},
			)
		case "textDocument/definition":
			result = []any{
				map[string]any{
					"targetUri": "file:///repo/b.go",
					"targetSelectionRange": map[string]any{
						"start": map[string]any{"line": 9, "character": 5},
					},
				},
			}
		case "textDocument/references":
			result = []any{
				map[string]any{
					"uri":   "file:///repo/a.go",
					"range": map[string]any{"start": map[string]any{"line": 2, "character": 1}},
				},
				map[string]any{
					"uri":   "file:///repo/c.go",
					"range": map[string]any{"start": map[string]any{"line": 4, "character": 0}},
				},
			}
		case "textDocument/hover":
			result = map[string]any{
				"contents": map[string]any{
					"kind":  "markdown",
					"value": "```go\nfunc Transfer(a, b int) int\n```",
				},
			}
		case "shutdown":
			result = nil
		}
		_ = writeMessage(s.out, map[string]any{"jsonrpc": "2.0", "id": msg.ID, "result": result})
	}
}

func TestClient(t *testing.T) {
	cr, sw := io.Pipe()
	sr, cw := io.Pipe()
	srv := &fakeServer{in: bufio.NewReader(sr), out: sw, t: t}
	go srv.serve()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c := NewClient(cr, cw)
	if err := c.Initialize(ctx, "/repo"); err != nil {
		t.Fatal(err)
	}
	if err := c.DidOpen("/repo/a.go", "go", "package a\n"); err != nil {
		t.Fatal(err)
	}
	defs, err := c.Locate(ctx, "definition", "/repo/a.go", 2, 3)
	if err != nil || len(defs) != 1 || defs[0].Path != "/repo/b.go" || defs[0].Line != 9 ||
		defs[0].Char != 5 {
		t.Fatalf("Definition = %+v, %v", defs, err)
	}
	refs, err := c.References(ctx, "/repo/a.go", 2, 3)
	if err != nil || len(refs) != 2 || refs[1].Path != "/repo/c.go" || refs[1].Line != 4 {
		t.Fatalf("References = %+v, %v", refs, err)
	}
	hover, err := c.Hover(ctx, "/repo/a.go", 2, 3)
	if err != nil || hover != "func Transfer(a, b int) int" {
		t.Fatalf("Hover = %q, %v", hover, err)
	}
	if err := c.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestUTF16Column(t *testing.T) {
	tests := []struct {
		line    string
		display int
		want    int
	}{
		{"\tx := 1", 4, 1},
		{"\t\tfoo()", 9, 3},
		{"a := \"héllo\" + b", 15, 15},
		{"s := \"😀\" + b", 11, 12},
	}
	for _, tt := range tests {
		if got := UTF16Column(tt.line, tt.display, 4); got != tt.want {
			t.Errorf("UTF16Column(%q, %d) = %d, want %d", tt.line, tt.display, got, tt.want)
		}
	}
}

func TestDeadServerFailsCallsAtOnce(t *testing.T) {
	c := NewClient(strings.NewReader(""), io.Discard)
	deadline := time.Now().Add(time.Second)
	for c.Dead() == nil && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if c.Dead() == nil {
		t.Fatal("EOF from the server must mark the client dead")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := time.Now()
	if _, err := c.Hover(ctx, "/a.go", 0, 0); err == nil || time.Since(start) > time.Second {
		t.Fatalf("Hover on a dead client = %v after %s, want an immediate error", err, time.Since(start))
	}
}
