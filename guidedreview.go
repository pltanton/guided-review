package guidedreview

import (
	_ "embed"
	"strings"
)

const Version = "0.1.1"

//go:embed CHANGELOG.md
var Changelog string

func WhatsNew(version string) string {
	_, rest, ok := strings.Cut(Changelog, "\n## v"+version+"\n")
	if !ok {
		return ""
	}
	section, _, _ := strings.Cut(rest, "\n## ")
	return strings.TrimSpace(section)
}
