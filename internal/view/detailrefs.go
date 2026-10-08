package view

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

const maxDetailRefs = 9

var refPattern = regexp.MustCompile(`(?:^|[\s(\x60\[])([\w][\w./-]*\.[A-Za-z]+):(\d+)`)

func (m *model) detailRefs(text string) []lspLoc {
	var refs []lspLoc
	seen := map[string]bool{}
	for _, g := range refPattern.FindAllStringSubmatch(text, -1) {
		path, ok := m.resolveRef(g[1])
		n, _ := strconv.Atoi(g[2])
		key := fmt.Sprintf("%s:%d", path, n)
		if !ok || n < 1 || seen[key] {
			continue
		}
		seen[key] = true
		refs = append(refs, lspLoc{Path: path, Line: n})
		if len(refs) == maxDetailRefs {
			break
		}
	}
	return refs
}

func (m *model) resolveRef(path string) (string, bool) {
	for _, r := range m.rows {
		if r.File == path || strings.HasSuffix(r.File, "/"+path) {
			return r.File, true
		}
	}
	if _, err := os.Stat(filepath.Join(m.codeDir(), path)); err == nil {
		return path, true
	}
	return "", false
}

func (m *model) refLines(p *popup, width int) []string {
	if len(p.refs) == 0 {
		return nil
	}
	out := []string{"", dimStyle.Render("code it mentions · 1-9 open")}
	for i, loc := range p.refs {
		head := fmt.Sprintf("%d  %s:%d", i+1, loc.Path, loc.Line)
		out = append(out, "", hotStyle.Render(head))
		lines := m.peek(loc.Path)
		target := loc.Line - 1
		for _, l := range markedCodeLines(lines, max(target-1, 0), target, 3, nil, 0) {
			out = append(out, ansi.Truncate(l, width, ""))
		}
	}
	return out
}
