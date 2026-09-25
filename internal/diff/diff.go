package diff

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type Status string

const (
	Added    Status = "added"
	Deleted  Status = "deleted"
	Modified Status = "modified"
	Renamed  Status = "renamed"
)

type File struct {
	Path    string
	OldPath string
	Status  Status
	Binary  bool
	Hunks   []Hunk
}

type Hunk struct {
	OldStart, OldLines int
	NewStart, NewLines int
	Lines              []Line
}

type Line struct {
	Kind byte
	Text string
}

func (h Hunk) NewEnd() int {
	if h.NewLines == 0 {
		return h.NewStart
	}
	return h.NewStart + h.NewLines - 1
}

func (h Hunk) Range() string {
	switch h.NewLines {
	case 0:
		return fmt.Sprintf("%d(del)", h.NewStart)
	case 1:
		return strconv.Itoa(h.NewStart)
	default:
		return fmt.Sprintf("%d-%d", h.NewStart, h.NewEnd())
	}
}

func (f File) Stat() (added, deleted int) {
	for _, h := range f.Hunks {
		for _, l := range h.Lines {
			if l.Kind == '+' {
				added++
			} else {
				deleted++
			}
		}
	}
	return added, deleted
}

var hunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

func Parse(s string) ([]File, error) {
	var (
		files []File
		cur   *File
		hunk  *Hunk
	)
	flushHunk := func() {
		if hunk != nil {
			cur.Hunks = append(cur.Hunks, *hunk)
			hunk = nil
		}
	}
	flushFile := func() {
		if cur != nil {
			flushHunk()
			files = append(files, *cur)
			cur = nil
		}
	}
	for _, line := range strings.Split(s, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			flushFile()
			cur = &File{Path: headerPath(line), Status: Modified}
		case cur == nil:
		case hunk != nil && (strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-")):
			hunk.Lines = append(hunk.Lines, Line{Kind: line[0], Text: line[1:]})
		case strings.HasPrefix(line, "@@"):
			flushHunk()
			h, err := parseHunkHeader(line)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", cur.Path, err)
			}
			hunk = &h
		case strings.HasPrefix(line, "new file mode"):
			cur.Status = Added
		case strings.HasPrefix(line, "deleted file mode"):
			cur.Status = Deleted
		case strings.HasPrefix(line, "rename from "):
			cur.OldPath = strings.TrimPrefix(line, "rename from ")
			cur.Status = Renamed
		case strings.HasPrefix(line, "rename to "):
			cur.Path = strings.TrimPrefix(line, "rename to ")
		case strings.HasPrefix(line, "Binary files "):
			cur.Binary = true
		}
	}
	flushFile()
	return files, nil
}

func headerPath(line string) string {
	rest := strings.TrimPrefix(line, "diff --git ")
	if i := strings.LastIndex(rest, " b/"); i >= 0 {
		return rest[i+3:]
	}
	return rest
}

func parseHunkHeader(line string) (Hunk, error) {
	m := hunkHeader.FindStringSubmatch(line)
	if m == nil {
		return Hunk{}, fmt.Errorf("bad hunk header %q", line)
	}
	n := func(s string) int {
		if s == "" {
			return 1
		}
		v, _ := strconv.Atoi(s)
		return v
	}
	return Hunk{OldStart: n(m[1]), OldLines: n(m[2]), NewStart: n(m[3]), NewLines: n(m[4])}, nil
}

func (f File) OldLineFor(n int) (old int, added bool) {
	delta := 0
	for _, h := range f.Hunks {
		if h.NewLines > 0 && n >= h.NewStart && n <= h.NewEnd() {
			return 0, true
		}
		if h.NewLines == 0 && h.NewStart < n || h.NewLines > 0 && h.NewEnd() < n {
			delta += h.NewLines - h.OldLines
		}
	}
	return n - delta, false
}
