package plan

import (
	"fmt"

	"github.com/pltanton/guided-review/internal/diff"
	"github.com/pltanton/guided-review/internal/state"
)

func Fill(r *state.Review, p Plan, files []diff.File) []error {
	var errs []error
	fail := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }
	inDiff := map[string]bool{}
	for _, f := range files {
		inDiff[f.Path] = true
	}
	for _, ps := range p.Steps {
		i := r.StepIndex(ps.ID)
		if i < 0 {
			fail("step %s: not in the plan", ps.ID)
			continue
		}
		filled := r.Steps[i]
		filled.Intro, filled.Annotations, filled.Hotspots = ps.Intro, ps.Annotations, ps.Hotspots
		prev := ""
		if i > 0 {
			prev = r.Steps[i-1].Chapter
		}
		for _, err := range checkExplanations(filled, prev, inDiff) {
			fail("step %s: %v", ps.ID, err)
		}
	}
	if len(errs) > 0 {
		return errs
	}
	for _, ps := range p.Steps {
		st := r.Step(ps.ID)
		if ps.Intro != "" {
			st.Intro = ps.Intro
		}
		if ps.Message != "" {
			st.Message = ps.Message
		}
		if len(ps.Annotations) > 0 {
			st.Annotations = ps.Annotations
		}
		if len(ps.Hotspots) > 0 {
			st.Hotspots = ps.Hotspots
			for j, h := range st.Hotspots {
				st.Hotspots[j].File, _ = st.HotspotFile(h)
			}
		}
	}
	return nil
}
