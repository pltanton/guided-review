package plan

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/aplotnikov/guided-review/internal/state"
)

var ErrDone = errors.New("no pending steps left")

func Next(r *state.Review) (*state.Step, error) {
	if cur := r.Step(r.Current); cur != nil && cur.Status == state.StatusPending {
		cur.Status = state.StatusDone
	}
	return advance(r)
}

func Skip(r *state.Review, reason string) (*state.Step, error) {
	if strings.TrimSpace(reason) == "" {
		return nil, errors.New("skip needs a reason")
	}
	cur := r.Step(r.Current)
	if cur == nil {
		return nil, errors.New("no current step")
	}
	cur.Status = state.StatusSkipped
	cur.SkipReason = reason
	return advance(r)
}

func Goto(r *state.Review, id string) error {
	if r.Step(id) == nil {
		return fmt.Errorf("no step %q", id)
	}
	r.Current = id
	return nil
}

func advance(r *state.Review) (*state.Step, error) {
	start := r.StepIndex(r.Current)
	n := len(r.Steps)
	for i := 1; i <= n; i++ {
		s := &r.Steps[(start+i+n)%n]
		if s.Status == state.StatusPending {
			r.Current = s.ID
			return s, nil
		}
	}
	return nil, ErrDone
}

func Dependents(r *state.Review, id string) []string {
	reverse := map[string][]string{}
	for _, s := range r.Steps {
		for _, d := range s.DependsOn {
			reverse[d] = append(reverse[d], s.ID)
		}
	}
	seen := map[string]bool{id: true}
	queue := []string{id}
	var out []string
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, d := range reverse[cur] {
			if !seen[d] {
				seen[d] = true
				out = append(out, d)
				queue = append(queue, d)
			}
		}
	}
	slices.SortFunc(out, func(a, b string) int { return r.StepIndex(a) - r.StepIndex(b) })
	return out
}

type Impact struct {
	Stale     []string
	MayChange []string
}

func AddComment(r *state.Review, c state.Comment) (state.Comment, Impact, error) {
	if !slices.Contains(state.Severities, c.Severity) {
		return c, Impact{}, fmt.Errorf("severity %q, want one of %v", c.Severity, state.Severities)
	}
	if strings.TrimSpace(c.Body) == "" {
		return c, Impact{}, errors.New("empty comment")
	}
	if r.File(c.File) == nil {
		return c, Impact{}, fmt.Errorf("%s: not in diff", c.File)
	}
	if c.Lines == "" {
		return c, Impact{}, errors.New("comment needs lines")
	}
	if _, _, err := state.ParseLines(c.Lines); err != nil {
		return c, Impact{}, err
	}
	if c.Step == "" {
		c.Step = r.Current
	}
	if r.Step(c.Step) == nil {
		return c, Impact{}, fmt.Errorf("no step %q", c.Step)
	}
	if c.SHA == "" {
		c.SHA = r.HeadSHA
	}
	c.Round = max(r.Round, 1)
	for _, existing := range r.Comments {
		c.ID = max(c.ID, existing.ID)
	}
	c.ID++
	r.Comments = append(r.Comments, c)

	var imp Impact
	if c.Severity != state.SeverityBlocker && c.Severity != state.SeverityMajor {
		return c, imp, nil
	}
	for _, id := range Dependents(r, c.Step) {
		s := r.Step(id)
		if s.Status != state.StatusPending {
			continue
		}
		if c.Severity == state.SeverityBlocker && len(s.Hotspots) == 0 {
			s.Status = state.StatusStale
			imp.Stale = append(imp.Stale, id)
			continue
		}
		s.MayChange = true
		imp.MayChange = append(imp.MayChange, id)
	}
	return c, imp, nil
}

func Gate(r *state.Review) []string {
	var pending []string
	for _, s := range r.Steps {
		if s.Status == state.StatusPending {
			pending = append(pending, s.ID)
		}
	}
	return pending
}

type Coverage struct {
	Total, Done, Skipped, Stale, Pending int
	Hotspots, HotspotsReviewed           int
	Boilerplate, Generated               int
}

func CoverageOf(r *state.Review) Coverage {
	var c Coverage
	for _, s := range r.Steps {
		c.Total++
		switch s.Status {
		case state.StatusDone:
			c.Done++
		case state.StatusSkipped:
			c.Skipped++
		case state.StatusStale:
			c.Stale++
		default:
			c.Pending++
		}
		c.Hotspots += len(s.Hotspots)
		if s.Status == state.StatusDone || s.Status == state.StatusSkipped {
			c.HotspotsReviewed += len(s.Hotspots)
		}
	}
	for _, f := range r.Files {
		switch f.Tier {
		case state.TierBoilerplate:
			c.Boilerplate++
		case state.TierGenerated:
			c.Generated++
		}
	}
	return c
}

func AddNote(r *state.Review, stepID string, a state.Annotation) error {
	if stepID == "" {
		stepID = r.Current
	}
	st := r.Step(stepID)
	if st == nil {
		return fmt.Errorf("no step %q", stepID)
	}
	if a.Kind == "" {
		a.Kind = "note"
	}
	inDiff := map[string]bool{}
	for _, f := range r.Files {
		inDiff[f.Path] = true
	}
	if err := checkAnnotation(a, inDiff); err != nil {
		return err
	}
	st.Annotations = append(st.Annotations, a)
	return nil
}

func ResolveComment(r *state.Review, id int) error {
	for i := range r.Comments {
		if r.Comments[i].ID == id {
			r.Comments[i].Resolved = true
			return nil
		}
	}
	return fmt.Errorf("no comment #%d", id)
}
