package plan

import (
	"fmt"
	"slices"
	"strings"

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
		st := r.Steps[i]
		if ps.Intro != "" && st.Chapter != "" && i > 0 && r.Steps[i-1].Chapter == st.Chapter {
			fail("step %s: intro belongs on the first step of chapter %q", ps.ID, st.Chapter)
		}
		for _, a := range ps.Annotations {
			if err := checkAnnotation(a, inDiff); err != nil {
				fail("step %s: %v", ps.ID, err)
			}
		}
		for _, h := range ps.Hotspots {
			if _, sure := st.HotspotFile(h); h.Line > 0 && !sure {
				fail("step %s: hotspot at line %d needs file: several files in the step",
					ps.ID, h.Line)
			}
			if !slices.Contains(state.HotspotCategories, h.Cat) {
				fail("step %s: hotspot category %q, want one of %v", ps.ID, h.Cat,
					state.HotspotCategories)
			}
			if strings.TrimSpace(h.Q) == "" {
				fail("step %s: hotspot %s has no question", ps.ID, h.Cat)
			}
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
