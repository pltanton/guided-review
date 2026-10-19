package view

import (
	"path/filepath"
	"strings"
	"sync"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/lipgloss"
)

var styleName = defaultTones.chroma

// lexers.Match globs every lexer's filenames: 83% of a reload on a 900-row step went there.
var lexerCache sync.Map

func lexerFor(path string) chroma.Lexer {
	name := filepath.Base(path)
	if l, ok := lexerCache.Load(name); ok {
		lexer, _ := l.(chroma.Lexer)
		return lexer
	}
	lexer := lexers.Match(name)
	lexerCache.Store(name, lexer)
	return lexer
}

func Highlight(path, content string) []string {
	plain := splitLines(content)
	lexer := lexerFor(path)
	if lexer == nil || plain == nil {
		return plain
	}
	it, err := chroma.Coalesce(lexer).Tokenise(nil, content)
	if err != nil {
		return plain
	}
	style := styles.Get(styleName)
	if style == nil {
		style = styles.Fallback
	}
	cache := map[chroma.TokenType]lipgloss.Style{}
	out := make([]string, 0, len(plain))
	var b strings.Builder
	for tok := it(); tok != chroma.EOF; tok = it() {
		st, ok := cache[tok.Type]
		if !ok {
			st = tokenStyle(style, tok.Type)
			cache[tok.Type] = st
		}
		for i, part := range strings.Split(tok.Value, "\n") {
			if i > 0 {
				out = append(out, b.String())
				b.Reset()
			}
			if part != "" {
				b.WriteString(st.Render(part))
			}
		}
	}
	if b.Len() > 0 {
		out = append(out, b.String())
	}
	if len(out) != len(plain) {
		return plain
	}
	return out
}

func tokenStyle(s *chroma.Style, t chroma.TokenType) lipgloss.Style {
	e := s.Get(t)
	st := lipgloss.NewStyle()
	if e.Colour.IsSet() {
		st = st.Foreground(lipgloss.Color(e.Colour.String()))
	}
	if e.Bold == chroma.Yes {
		st = st.Bold(true)
	}
	return st
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

func expandTabs(s string) string {
	return strings.ReplaceAll(s, "\t", "    ")
}
