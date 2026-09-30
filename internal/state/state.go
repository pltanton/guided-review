package state

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

type Tier string

const (
	TierCore        Tier = "core"
	TierBoilerplate Tier = "boilerplate"
	TierGenerated   Tier = "generated"
)

type StepStatus string

const (
	StatusPending StepStatus = "pending"
	StatusDone    StepStatus = "done"
	StatusSkipped StepStatus = "skipped"
	StatusStale   StepStatus = "stale"
)

func (s StepStatus) Glyph() string {
	switch s {
	case StatusDone:
		return "✓"
	case StatusSkipped:
		return "↷"
	case StatusStale:
		return "~"
	}
	return "·"
}

type Severity string

const (
	SeverityBlocker Severity = "blocker"
	SeverityMajor   Severity = "major"
	SeverityMinor   Severity = "minor"
	SeverityNit     Severity = "nit"
)

var (
	Severities        = []Severity{SeverityBlocker, SeverityMajor, SeverityMinor, SeverityNit}
	HotspotCategories = []string{"security", "consistency", "money", "migration"}
	AnnotationKinds   = []string{"note", "spec"}
)

type Review struct {
	ID           string       `yaml:"id"`
	Source       string       `yaml:"source"`
	BaseSHA      string       `yaml:"base_sha"`
	StartSHA     string       `yaml:"start_sha,omitempty"`
	HeadSHA      string       `yaml:"head_sha"`
	MR           *MR          `yaml:"mr,omitempty"`
	Worktree     string       `yaml:"worktree,omitempty"`
	Mode         string       `yaml:"mode,omitempty"`
	Round        int          `yaml:"round,omitempty"`
	RoundStart   time.Time    `yaml:"round_start,omitempty"`
	PrevHeadSHA  string       `yaml:"prev_head_sha,omitempty"`
	RoundBaseSHA string       `yaml:"round_base_sha,omitempty"`
	RoundRebased bool         `yaml:"round_rebased,omitempty"`
	RoundFiles   []string     `yaml:"round_files,omitempty"`
	Domain       string       `yaml:"domain,omitempty"`
	Files        []File       `yaml:"files"`
	Summary      string       `yaml:"summary,omitempty"`
	Steps        []Step       `yaml:"steps,omitempty"`
	Current      string       `yaml:"current,omitempty"`
	Comments     []Comment    `yaml:"comments,omitempty"`
	Discussions  []Discussion `yaml:"discussions,omitempty"`
	Messages     []Message    `yaml:"messages,omitempty"`
	Progress     *Progress    `yaml:"progress,omitempty"`
	SummaryRound int          `yaml:"summary_round,omitempty"`
	Publish      *PublishPlan `yaml:"publish,omitempty"`
}

type Message struct {
	Time    time.Time `yaml:"time"`
	Step    string    `yaml:"step,omitempty"`
	Text    string    `yaml:"text"`
	Options []string  `yaml:"options,omitempty"`
}

const MaxMessages = 50

type PublishPlan struct {
	Verdict   string `yaml:"verdict"`
	Decisions string `yaml:"decisions,omitempty"`
	Approve   bool   `yaml:"approve,omitempty"`
}

type Progress struct {
	Text string    `yaml:"text"`
	Time time.Time `yaml:"time"`
}

type Discussion struct {
	ID       string `yaml:"id"`
	Author   string `yaml:"author"`
	Body     string `yaml:"body"`
	Replies  int    `yaml:"replies,omitempty"`
	File     string `yaml:"file,omitempty"`
	Line     int    `yaml:"line,omitempty"`
	OldLine  bool   `yaml:"old_line,omitempty"`
	Resolved bool   `yaml:"resolved,omitempty"`
}

type MR struct {
	Provider string `yaml:"provider,omitempty"`
	URL      string `yaml:"url"`
	Host     string `yaml:"host"`
	Project  string `yaml:"project"`
	IID      int    `yaml:"iid"`
	Title    string `yaml:"title"`
}

type File struct {
	Path    string `yaml:"path"`
	OldPath string `yaml:"old_path,omitempty"`
	Status  string `yaml:"status"`
	Tier    Tier   `yaml:"tier"`
	Added   int    `yaml:"added"`
	Deleted int    `yaml:"deleted"`
}

type Step struct {
	ID          string       `yaml:"id"`
	Title       string       `yaml:"title"`
	Kind        string       `yaml:"kind"`
	Chapter     string       `yaml:"chapter,omitempty"`
	Intro       string       `yaml:"intro,omitempty"`
	Hunks       []StepHunk   `yaml:"hunks"`
	Hotspots    []Hotspot    `yaml:"hotspots,omitempty"`
	DependsOn   []string     `yaml:"depends_on,omitempty"`
	Annotations []Annotation `yaml:"annotations,omitempty"`
	Details     []Detail     `yaml:"details,omitempty"`
	Note        string       `yaml:"note,omitempty"`
	Message     string       `yaml:"message,omitempty"`
	WhyBig      string       `yaml:"why_big,omitempty"`
	Announced   bool         `yaml:"announced,omitempty"`
	Status      StepStatus   `yaml:"status,omitempty"`
	MayChange   bool         `yaml:"may_change,omitempty"`
	SkipReason  string       `yaml:"skip_reason,omitempty"`
}

type StepHunk struct {
	File  string `yaml:"file"`
	Lines string `yaml:"lines,omitempty"`
}

type Hotspot struct {
	Cat    string `yaml:"cat"`
	Q      string `yaml:"q"`
	File   string `yaml:"file,omitempty"`
	Line   int    `yaml:"line,omitempty"`
	Detail string `yaml:"detail,omitempty"`
}

type Detail struct {
	File string `yaml:"file"`
	Line int    `yaml:"line"`
	Text string `yaml:"text"`
}

type Annotation struct {
	File   string `yaml:"file"`
	Line   int    `yaml:"line"`
	To     int    `yaml:"to,omitempty"`
	Kind   string `yaml:"kind"`
	Text   string `yaml:"text"`
	Detail string `yaml:"detail,omitempty"`
}

type Comment struct {
	ID         int      `yaml:"id"`
	Step       string   `yaml:"step"`
	File       string   `yaml:"file"`
	Lines      string   `yaml:"lines"`
	SHA        string   `yaml:"sha"`
	Severity   Severity `yaml:"severity"`
	Body       string   `yaml:"body"`
	Suggestion string   `yaml:"suggestion,omitempty"`
	Round      int      `yaml:"round,omitempty"`
	Resolved   bool     `yaml:"resolved,omitempty"`
	Published  bool     `yaml:"published,omitempty"`
}

func (r *Review) CodeDir(repoDir string) string {
	if r.Worktree != "" {
		return r.Worktree
	}
	return repoDir
}

func (r *Review) DiffBase() string {
	if r.RoundBaseSHA != "" {
		return r.RoundBaseSHA
	}
	return r.BaseSHA
}

func (r *Review) Step(id string) *Step {
	if i := r.StepIndex(id); i >= 0 {
		return &r.Steps[i]
	}
	return nil
}

func (r *Review) StepIndex(id string) int {
	for i := range r.Steps {
		if r.Steps[i].ID == id {
			return i
		}
	}
	return -1
}

func (r *Review) File(path string) *File {
	for i := range r.Files {
		if r.Files[i].Path == path {
			return &r.Files[i]
		}
	}
	return nil
}

func ParseLines(s string) (start, end int, err error) {
	if s == "" {
		return 0, 0, nil
	}
	bad := fmt.Errorf("lines %q: want N or N-M with 1 <= N <= M", s)
	a, b, found := strings.Cut(s, "-")
	if start, err = strconv.Atoi(a); err != nil {
		return 0, 0, bad
	}
	end = start
	if found {
		if end, err = strconv.Atoi(b); err != nil {
			return 0, 0, bad
		}
	}
	if start < 1 || end < start {
		return 0, 0, bad
	}
	return start, end, nil
}

func (s *Step) Detail(file string, line int) (string, bool) {
	for _, d := range s.Details {
		if (d.File == file || d.File == "") && d.Line == line {
			return d.Text, true
		}
	}
	return "", false
}

func (s *Step) HotspotFile(h Hotspot) (file string, sure bool) {
	if h.File != "" || len(s.Hunks) == 0 {
		return h.File, true
	}
	var files []string
	for _, sh := range s.Hunks {
		if from, to, err := ParseLines(sh.Lines); err == nil && from <= h.Line && h.Line <= to {
			return sh.File, true
		}
		if !slices.Contains(files, sh.File) {
			files = append(files, sh.File)
		}
	}
	return files[0], len(files) == 1
}

const ProviderGitHub = "github"

func (m *MR) Label() string {
	if m.Provider == ProviderGitHub {
		return fmt.Sprintf("PR #%d", m.IID)
	}
	return fmt.Sprintf("MR !%d", m.IID)
}
