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

const markerLines = 40

var commentPrefixes = []string{"//", "#", "/*", "*", "--", "<!--"}

type Classifier struct {
	res []*regexp.Regexp
}

func New(patternSets ...[]string) *Classifier {
	c := &Classifier{}
	for _, set := range patternSets {
		for _, p := range set {
			c.res = append(c.res, globToRegexp(p))
		}
	}
	return c
}

func (c *Classifier) Generated(path string) bool {
	for _, re := range c.res {
		if re.MatchString(path) {
			return true
		}
	}
	return false
}

func HasMarker(content string) bool {
	for i, line := range strings.SplitN(content, "\n", markerLines+1) {
		if i == markerLines {
			break
		}
		line = strings.TrimSpace(line)
		if !hasCommentPrefix(line) {
			continue
		}
		if strings.Contains(line, "DO NOT EDIT") || strings.Contains(line, "@generated") {
			return true
		}
	}
	return false
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
	for _, line := range strings.Split(s, "\n") {
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
