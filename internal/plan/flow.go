package plan

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/pltanton/guided-review/internal/state"
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
	Restored  []string
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
	checkCommentedHotspots(r, c)
	return c, recomputeStale(r), nil
}

func checkCommentedHotspots(r *state.Review, c state.Comment) {
	from, to, err := state.ParseLines(c.Lines)
	if err != nil {
		return
	}
	for si := range r.Steps {
		st := &r.Steps[si]
		for i, h := range st.Hotspots {
			if file, _ := st.HotspotFile(h); h.Line >= from && h.Line <= max(to, from) && file == c.File {
				st.Hotspots[i].Checked, st.Hotspots[i].Comment = true, c.ID
			}
		}
	}
}

func recomputeStale(r *state.Review) Impact {
	round := max(r.Round, 1)
	blocked, affected := map[string]bool{}, map[string]bool{}
	for _, c := range r.Comments {
		weighty := c.Severity == state.SeverityBlocker || c.Severity == state.SeverityMajor
		if c.Resolved || !weighty || max(c.Round, 1) != round {
			continue
		}
		for _, d := range Dependents(r, c.Step) {
			affected[d] = true
			blocked[d] = blocked[d] || c.Severity == state.SeverityBlocker
		}
	}
	var imp Impact
	for i := range r.Steps {
		s := &r.Steps[i]
		switch {
		case s.Status == state.StatusStale && !blocked[s.ID]:
			s.Status = state.StatusPending
			imp.Restored = append(imp.Restored, s.ID)
		case s.Status == state.StatusPending && blocked[s.ID] && len(s.Hotspots) == 0:
			s.Status = state.StatusStale
			imp.Stale = append(imp.Stale, s.ID)
		}
		switch {
		case !affected[s.ID]:
			s.MayChange = false
		case s.Status == state.StatusPending && !s.MayChange:
			s.MayChange = true
			imp.MayChange = append(imp.MayChange, s.ID)
		}
	}
	return imp
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
		for _, h := range s.Hotspots {
			if h.Checked {
				c.HotspotsReviewed++
			}
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

func SetDetail(r *state.Review, stepID string, d state.Detail) error {
	id := cmp.Or(stepID, r.Current)
	st := r.Step(id)
	switch {
	case st == nil:
		return fmt.Errorf("no step %q", id)
	case d.File == "" || d.Line < 1:
		return errors.New("a detail needs --file and --line of the note it explains")
	case strings.TrimSpace(d.Text) == "":
		return errors.New("empty detail")
	case !st.SetDetail(d.File, d.Line, d.Text):
		return fmt.Errorf("step %s: no note ends at %s:%d: add it with gr note add first",
			id, d.File, d.Line)
	}
	return nil
}

func ResolveComment(r *state.Review, id int) (Impact, error) {
	for i := range r.Comments {
		if r.Comments[i].ID == id {
			r.Comments[i].Resolved = true
			return recomputeStale(r), nil
		}
	}
	return Impact{}, fmt.Errorf("no comment #%d", id)
}

func DeleteComment(r *state.Review, id int) (Impact, error) {
	i := slices.IndexFunc(r.Comments, func(c state.Comment) bool { return c.ID == id })
	switch {
	case i < 0:
		return Impact{}, fmt.Errorf("no comment #%d", id)
	case r.Comments[i].Published:
		return Impact{}, fmt.Errorf("comment #%d is already on the MR: delete it there", id)
	}
	r.Comments = slices.Delete(r.Comments, i, i+1)
	return recomputeStale(r), nil
}

func EditComment(r *state.Review, id int, body string, severity state.Severity) (Impact, error) {
	if strings.TrimSpace(body) == "" {
		return Impact{}, errors.New("empty comment")
	}
	if severity != "" && !slices.Contains(state.Severities, severity) {
		return Impact{}, fmt.Errorf("severity %q, want one of %v", severity, state.Severities)
	}
	for i := range r.Comments {
		if r.Comments[i].ID != id {
			continue
		}
		r.Comments[i].Body = body
		if severity != "" {
			r.Comments[i].Severity = severity
		}
		return recomputeStale(r), nil
	}
	return Impact{}, fmt.Errorf("no comment #%d", id)
}

func CheckHotspot(r *state.Review, stepID string, n int, checked bool) error {
	st := r.Step(stepID)
	if st == nil {
		return fmt.Errorf("no step %q", stepID)
	}
	if n < 1 || n > len(st.Hotspots) {
		return fmt.Errorf("step %s has %d hotspots, no %d", stepID, len(st.Hotspots), n)
	}
	st.Hotspots[n-1].Checked = checked
	return nil
}
