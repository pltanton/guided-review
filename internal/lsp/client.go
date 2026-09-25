package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf16"
)

type Location struct {
	Path string
	Line int
	Char int
}

type Client struct {
	w       io.Writer
	wmu     sync.Mutex
	nextID  atomic.Int64
	mu      sync.Mutex
	pending map[int64]chan response
	done    chan struct{}
	cmd     *exec.Cmd
}

type response struct {
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error"`
}

type incoming struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	response
}

func NewClient(r io.Reader, w io.Writer) *Client {
	c := &Client{w: w, pending: map[int64]chan response{}, done: make(chan struct{})}
	go c.read(bufio.NewReader(r))
	return c
}

func Start(ctx context.Context, argv []string, root string) (*Client, error) {
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = root
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", argv[0], err)
	}
	c := NewClient(stdout, stdin)
	c.cmd = cmd
	if err := c.Initialize(ctx, root); err != nil {
		_ = cmd.Process.Kill()
		return nil, err
	}
	return c, nil
}

func (c *Client) read(r *bufio.Reader) {
	defer close(c.done)
	for {
		body, err := readMessage(r)
		if err != nil {
			c.failPending(err)
			return
		}
		var msg incoming
		if json.Unmarshal(body, &msg) != nil {
			continue
		}
		switch {
		case msg.Method != "" && len(msg.ID) > 0:
			go c.answerServer(msg)
		case msg.Method != "":
		default:
			id, err := strconv.ParseInt(string(msg.ID), 10, 64)
			if err != nil {
				continue
			}
			c.mu.Lock()
			ch := c.pending[id]
			delete(c.pending, id)
			c.mu.Unlock()
			if ch != nil {
				ch <- msg.response
			}
		}
	}
}

func (c *Client) failPending(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, ch := range c.pending {
		msg := err.Error()
		ch <- response{Error: &struct {
			Message string `json:"message"`
		}{msg}}
		delete(c.pending, id)
	}
}

func (c *Client) answerServer(msg incoming) {
	var result any
	if msg.Method == "workspace/configuration" {
		var p struct {
			Items []json.RawMessage `json:"items"`
		}
		_ = json.Unmarshal(msg.Params, &p)
		result = make([]any, len(p.Items))
	}
	_ = c.send(map[string]any{"jsonrpc": "2.0", "id": msg.ID, "result": result})
}

func (c *Client) send(v any) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	return writeMessage(c.w, v)
}

func (c *Client) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := c.nextID.Add(1)
	ch := make(chan response, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()
	if err := c.send(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, ctx.Err()
	case r := <-ch:
		if r.Error != nil {
			return nil, fmt.Errorf("%s: %s", method, r.Error.Message)
		}
		return r.Result, nil
	}
}

func (c *Client) notify(method string, params any) error {
	return c.send(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

func (c *Client) Initialize(ctx context.Context, root string) error {
	uri := fileURI(root)
	params := map[string]any{
		"processId": os.Getpid(),
		"rootUri":   uri,
		"capabilities": map[string]any{
			"textDocument": map[string]any{
				"hover":      map[string]any{"contentFormat": []string{"plaintext", "markdown"}},
				"definition": map[string]any{"linkSupport": true},
			},
			"workspace": map[string]any{"configuration": true, "workspaceFolders": true},
		},
		"workspaceFolders": []any{map[string]any{"uri": uri, "name": "review"}},
	}
	if _, err := c.call(ctx, "initialize", params); err != nil {
		return err
	}
	return c.notify("initialized", map[string]any{})
}

func (c *Client) DidOpen(path, language, text string) error {
	return c.notify("textDocument/didOpen", map[string]any{
		"textDocument": map[string]any{"uri": fileURI(path), "languageId": language, "version": 1, "text": text},
	})
}

func position(path string, line, char int) map[string]any {
	return map[string]any{
		"textDocument": map[string]any{"uri": fileURI(path)},
		"position":     map[string]any{"line": line, "character": char},
	}
}

type lspRange struct {
	Start struct {
		Line      int `json:"line"`
		Character int `json:"character"`
	} `json:"start"`
}

type rawLocation struct {
	URI                  string    `json:"uri"`
	Range                *lspRange `json:"range"`
	TargetURI            string    `json:"targetUri"`
	TargetRange          *lspRange `json:"targetRange"`
	TargetSelectionRange *lspRange `json:"targetSelectionRange"`
}

func (l rawLocation) location() Location {
	uri, rng := l.URI, l.Range
	if l.TargetURI != "" {
		uri, rng = l.TargetURI, l.TargetSelectionRange
		if rng == nil {
			rng = l.TargetRange
		}
	}
	loc := Location{Path: uriPath(uri)}
	if rng != nil {
		loc.Line, loc.Char = rng.Start.Line, rng.Start.Character
	}
	return loc
}

func parseLocations(raw json.RawMessage) ([]Location, error) {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return nil, nil
	}
	var many []rawLocation
	if strings.HasPrefix(s, "[") {
		if err := json.Unmarshal(raw, &many); err != nil {
			return nil, err
		}
	} else {
		var one rawLocation
		if err := json.Unmarshal(raw, &one); err != nil {
			return nil, err
		}
		many = []rawLocation{one}
	}
	out := make([]Location, 0, len(many))
	for _, l := range many {
		out = append(out, l.location())
	}
	return out, nil
}

func (c *Client) Definition(ctx context.Context, path string, line, char int) ([]Location, error) {
	raw, err := c.call(ctx, "textDocument/definition", position(path, line, char))
	if err != nil {
		return nil, err
	}
	return parseLocations(raw)
}

func (c *Client) References(ctx context.Context, path string, line, char int) ([]Location, error) {
	p := position(path, line, char)
	p["context"] = map[string]any{"includeDeclaration": false}
	raw, err := c.call(ctx, "textDocument/references", p)
	if err != nil {
		return nil, err
	}
	return parseLocations(raw)
}

func (c *Client) Hover(ctx context.Context, path string, line, char int) (string, error) {
	raw, err := c.call(ctx, "textDocument/hover", position(path, line, char))
	if err != nil {
		return "", err
	}
	var h struct {
		Contents json.RawMessage `json:"contents"`
	}
	if s := strings.TrimSpace(string(raw)); s == "" || s == "null" {
		return "", nil
	}
	if err := json.Unmarshal(raw, &h); err != nil {
		return "", err
	}
	return stripFences(markupText(h.Contents)), nil
}

func markupText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var obj struct {
		Value string `json:"value"`
	}
	if json.Unmarshal(raw, &obj) == nil && obj.Value != "" {
		return obj.Value
	}
	var arr []json.RawMessage
	if json.Unmarshal(raw, &arr) == nil {
		parts := make([]string, 0, len(arr))
		for _, a := range arr {
			parts = append(parts, markupText(a))
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

func stripFences(s string) string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "```") {
			continue
		}
		out = append(out, l)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func (c *Client) Shutdown(ctx context.Context) error {
	_, err := c.call(ctx, "shutdown", nil)
	_ = c.notify("exit", nil)
	if c.cmd != nil {
		exited := make(chan struct{})
		go func() {
			_ = c.cmd.Wait()
			close(exited)
		}()
		select {
		case <-exited:
		case <-time.After(2 * time.Second):
			_ = c.cmd.Process.Kill()
		}
	}
	return err
}

func readMessage(r *bufio.Reader) ([]byte, error) {
	length := -1
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if name, value, ok := strings.Cut(line, ":"); ok && strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			length, err = strconv.Atoi(strings.TrimSpace(value))
			if err != nil {
				return nil, err
			}
		}
	}
	if length < 0 {
		return nil, errors.New("lsp: message without Content-Length")
	}
	body := make([]byte, length)
	_, err := io.ReadFull(r, body)
	return body, err
}

func writeMessage(w io.Writer, v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "Content-Length: %d\r\n\r\n%s", len(body), body)
	return err
}

func fileURI(path string) string {
	return (&url.URL{Scheme: "file", Path: path}).String()
}

func uriPath(uri string) string {
	u, err := url.Parse(uri)
	if err != nil {
		return uri
	}
	return u.Path
}

func UTF16Column(line string, display, tabWidth int) int {
	col, units := 0, 0
	for _, r := range line {
		if col >= display {
			break
		}
		if r == '\t' {
			col += tabWidth
		} else {
			col++
		}
		units += utf16.RuneLen(r)
	}
	return units
}
