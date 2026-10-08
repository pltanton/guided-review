package classify

import (
	"regexp"
	"strings"
)

var DefaultPatterns = []string{
	"*.pb.go", "*.pb.gw.go", "*_pb2.py", "*_pb2.pyi", "*_pb2_grpc.py",
	"*.gen.go", "*_gen.go", "zz_generated*.go",
	"go.sum", "package-lock.json", "yarn.lock", "pnpm-lock.yaml",
	"poetry.lock", "uv.lock", "Cargo.lock", "gradle.lockfile",
	"**/build/generated/**", "vendor/**",
}

const (
	markerLines  = 40
	MarkerReason = "marker"
)

var (
	commentPrefixes = []string{"//", "#", "/*", "*", "--", "<!--"}
	goGenerated     = regexp.MustCompile(`^// Code generated .* DO NOT EDIT\.$`)
)

type Set struct {
	Source   string
	Patterns []string
}

type rule struct {
	reason string
	re     *regexp.Regexp
}

type Classifier struct {
	rules []rule
}

func New(sets ...Set) *Classifier {
	c := &Classifier{}
	for _, set := range sets {
		for _, p := range set.Patterns {
			c.rules = append(c.rules, rule{set.Source + " " + p, globToRegexp(p)})
		}
	}
	return c
}

func (c *Classifier) Match(path string) string {
	for _, r := range c.rules {
		if r.re.MatchString(path) {
			return r.reason
		}
	}
	return ""
}

func Marker(content string) string {
	first := true
	for i, line := range strings.SplitN(content, "\n", markerLines+1) {
		line = strings.TrimSpace(line)
		if i == markerLines || line != "" && !hasCommentPrefix(line) {
			break
		}
		if goGenerated.MatchString(line) {
			return MarkerReason + " " + line
		}
		if strings.Trim(line, "/#*-<!> ") == "" || strings.HasPrefix(line, "#!") {
			continue
		}
		if first && strings.Contains(line, "@generated") {
			return MarkerReason + " " + line
		}
		first = false
	}
	return ""
}

func hasCommentPrefix(line string) bool {
	for _, p := range commentPrefixes {
		if strings.HasPrefix(line, p) {
			return true
		}
	}
	return false
}

func GitattributesPatterns(s string) []string {
	var out []string
	for line := range strings.SplitSeq(s, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 || strings.HasPrefix(f[0], "#") {
			continue
		}
		for _, attr := range f[1:] {
			if attr == "linguist-generated" || attr == "linguist-generated=true" {
				out = append(out, f[0])
				break
			}
		}
	}
	return out
}

func globToRegexp(p string) *regexp.Regexp {
	anchored := strings.HasPrefix(p, "/")
	p = strings.TrimPrefix(p, "/")
	if !anchored && !strings.Contains(p, "/") {
		p = "**/" + p
	}
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(p); {
		switch {
		case strings.HasPrefix(p[i:], "**/"):
			b.WriteString("(?:.*/)?")
			i += 3
		case strings.HasPrefix(p[i:], "/**") && i+3 == len(p):
			b.WriteString("/.*")
			i += 3
		case p[i] == '*':
			b.WriteString("[^/]*")
			i++
		case p[i] == '?':
			b.WriteString("[^/]")
			i++
		default:
			b.WriteString(regexp.QuoteMeta(p[i : i+1]))
			i++
		}
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}
