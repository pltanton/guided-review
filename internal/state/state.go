package state

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Tier string

const (
	TierCore        Tier = "core"
	TierTests       Tier = "tests"
	TierBoilerplate Tier = "boilerplate"
	TierOther       Tier = "other"
	TierGenerated   Tier = "generated"
)

// Tiers in reading order; every tier after core is a folded group in the viewer.
var Tiers = []Tier{TierCore, TierTests, TierBoilerplate, TierOther, TierGenerated}

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
	Briefing     string       `yaml:"briefing,omitempty"`
	Notify       string       `yaml:"notify,omitempty"`
	Hotspots     []Hotspot    `yaml:"hotspots,omitempty"`
	Annotations  []Annotation `yaml:"annotations,omitempty"`
	Comments     []Comment    `yaml:"comments,omitempty"`
	Discussions  []Discussion `yaml:"discussions,omitempty"`
	Threads      []Thread     `yaml:"threads,omitempty"`
	Messages     []Message    `yaml:"messages,omitempty"`
	Progress     *Progress    `yaml:"progress,omitempty"`
	SummaryRound int          `yaml:"summary_round,omitempty"`
	Publish      *PublishPlan `yaml:"publish,omitempty"`
	Published    *PublishPlan `yaml:"published,omitempty"`
}

type Message struct {
	Time    time.Time `yaml:"time"`
	Text    string    `yaml:"text"`
	Options []string  `yaml:"options,omitempty"`
}

const MaxMessages = 50

type PublishPlan struct {
	Verdict     string `yaml:"verdict"`
	Decisions   string `yaml:"decisions,omitempty"`
	Approve     bool   `yaml:"approve,omitempty"`
	Export      string `yaml:"export,omitempty"`
	Approved    bool   `yaml:"approved,omitempty"`
	VerdictSent bool   `yaml:"verdict_sent,omitempty"`
}

type Progress struct {
	Text string    `yaml:"text"`
	Time time.Time `yaml:"time"`
}

type Discussion struct {
	ID         string `yaml:"id"`
	Author     string `yaml:"author"`
	Body       string `yaml:"body"`
	Replies    int    `yaml:"replies,omitempty"`
	File       string `yaml:"file,omitempty"`
	Line       int    `yaml:"line,omitempty"`
	OldLine    bool   `yaml:"old_line,omitempty"`
	Resolved   bool   `yaml:"resolved,omitempty"`
	Resolvable bool   `yaml:"resolvable,omitempty"`
	Notes      []Note `yaml:"notes,omitempty"`
	ReplyTo    int64  `yaml:"reply_to,omitempty"`
	Comment    int    `yaml:"comment,omitempty"`
}

type Note struct {
	Author string `yaml:"author"`
	Body   string `yaml:"body"`
}

const (
	VerdictResolve  = "resolve"
	VerdictOpen     = "open"
	VerdictNone     = "none"
	DecidedByViewer = "viewer"
)

type Thread struct {
	ID            string    `yaml:"id"`
	Notes         int       `yaml:"notes"`
	Assessment    string    `yaml:"assessment,omitempty"`
	Proposed      string    `yaml:"proposed,omitempty"`
	ProposedReply string    `yaml:"proposed_reply,omitempty"`
	Verdict       string    `yaml:"verdict,omitempty"`
	Reply         string    `yaml:"reply,omitempty"`
	DecidedAt     time.Time `yaml:"decided_at,omitempty"`
	DecidedBy     string    `yaml:"decided_by,omitempty"`
	Resolves      int       `yaml:"resolves,omitempty"`
	ReplyPosted   bool      `yaml:"reply_posted,omitempty"`
	Published     bool      `yaml:"published,omitempty"`
}

type MR struct {
	Provider   string `yaml:"provider,omitempty"`
	URL        string `yaml:"url"`
	Host       string `yaml:"host"`
	Project    string `yaml:"project"`
	IID        int    `yaml:"iid"`
	Title      string `yaml:"title"`
	Me         string `yaml:"me,omitempty"`
	Author     string `yaml:"author,omitempty"`
	AuthorName string `yaml:"author_name,omitempty"`
}

type File struct {
	Path    string `yaml:"path"`
	OldPath string `yaml:"old_path,omitempty"`
	Status  string `yaml:"status"`
	Tier    Tier   `yaml:"tier"`
	Reason  string `yaml:"reason,omitempty"`
	Added   int    `yaml:"added"`
	Deleted int    `yaml:"deleted"`
	Order   int    `yaml:"order,omitempty"`
	Why     string `yaml:"why,omitempty"`
	Read    bool   `yaml:"read,omitempty"`
	Seen    bool   `yaml:"seen,omitempty"`
}

type Hotspot struct {
	Cat     string `yaml:"cat"`
	Q       string `yaml:"q"`
	File    string `yaml:"file,omitempty"`
	Line    int    `yaml:"line,omitempty"`
	Detail  string `yaml:"detail,omitempty"`
	Checked bool   `yaml:"checked,omitempty"`
	Comment int    `yaml:"comment,omitempty"`
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

// Code is source text; yaml.v3 v3.0.1 writes a leading tab as a literal block it cannot read back.
type Code string

func (c Code) MarshalYAML() (any, error) {
	n := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: string(c)}
	if strings.HasPrefix(n.Value, "\t") {
		n.Style = yaml.DoubleQuotedStyle
	}
	return n, nil
}

type Comment struct {
	ID            int      `yaml:"id"`
	File          string   `yaml:"file"`
	Lines         string   `yaml:"lines"`
	SHA           string   `yaml:"sha"`
	Severity      Severity `yaml:"severity"`
	Body          string   `yaml:"body"`
	Suggestion    Code     `yaml:"suggestion,omitempty"`
	Round         int      `yaml:"round,omitempty"`
	Resolved      bool     `yaml:"resolved,omitempty"`
	Published     bool     `yaml:"published,omitempty"`
	ThreadID      string   `yaml:"thread_id,omitempty"`
	ResolvedRound int      `yaml:"resolved_round,omitempty"`
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

func (r *Review) InRound(path string) bool {
	if !r.RoundRebased && len(r.RoundFiles) == 0 {
		return true
	}
	return slices.Contains(r.RoundFiles, path)
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

func (r *Review) Detail(file string, line int) (string, bool) {
	for _, h := range r.Hotspots {
		if h.File == file && h.Line == line && h.Detail != "" {
			return h.Detail, true
		}
	}
	for _, a := range r.Annotations {
		if a.File == file && max(a.To, a.Line) == line && a.Detail != "" {
			return a.Detail, true
		}
	}
	return "", false
}

func tierRank(t Tier) int {
	return max(slices.Index(Tiers, t), 0)
}

func (r *Review) FileOrder() []File {
	files := slices.Clone(r.Files)
	slices.SortStableFunc(files, func(a, b File) int {
		if c := cmp.Compare(tierRank(a.Tier), tierRank(b.Tier)); c != 0 {
			return c
		}
		switch {
		case a.Order > 0 && b.Order > 0:
			return cmp.Compare(a.Order, b.Order)
		case a.Order > 0:
			return -1
		case b.Order > 0:
			return 1
		}
		return cmp.Compare(a.Path, b.Path)
	})
	return files
}

const ProviderGitHub = "github"

func (m *MR) Label() string {
	if m.Provider == ProviderGitHub {
		return fmt.Sprintf("PR #%d", m.IID)
	}
	return fmt.Sprintf("MR !%d", m.IID)
}
