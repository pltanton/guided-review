package plan

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/pltanton/guided-review/internal/diff"
	"github.com/pltanton/guided-review/internal/state"
)

func Carry(steps []state.Step, round int, since, full []diff.File) []state.Step {
	changed := map[string]string{}
	for _, f := range since {
		changed[cmp.Or(f.OldPath, f.Path)] = f.Path
	}
	inFull := map[string]bool{}
	for _, f := range full {
		inFull[f.Path] = true
	}
	ids := map[string]string{}
	var out []state.Step
	for _, st := range steps {
		if !Unreviewed(st) {
			continue
		}
		var hunks []state.StepHunk
		for _, h := range st.Hunks {
			if path, ok := changed[h.File]; ok {
				h = state.StepHunk{File: path}
			}
			if inFull[h.File] && !slices.Contains(hunks, h) {
				hunks = append(hunks, h)
			}
		}
		if len(hunks) == 0 {
			continue
		}
		st.Hunks = hunks
		st.Annotations = slices.DeleteFunc(slices.Clone(st.Annotations),
			func(a state.Annotation) bool { return changed[a.File] != "" })
		st.Details = slices.DeleteFunc(slices.Clone(st.Details),
			func(d state.Detail) bool { return changed[d.File] != "" })
		st.Hotspots = slices.Clone(st.Hotspots)
		for i, h := range st.Hotspots {
			if path, ok := changed[h.File]; ok {
				st.Hotspots[i].File, st.Hotspots[i].Line = path, 0
			}
		}
		if st.FromRound == 0 {
			ids[st.ID] = fmt.Sprintf("r%d-%s", round, st.ID)
		} else {
			ids[st.ID] = st.ID
		}
		st.ID, st.FromRound = ids[st.ID], cmp.Or(st.FromRound, round)
		st.Status, st.MayChange, st.SkipReason, st.Announced = state.StatusPending, false, "", false
		out = append(out, st)
	}
	for i := range out {
		var deps []string
		for _, d := range out[i].DependsOn {
			if id, ok := ids[d]; ok {
				deps = append(deps, id)
			}
		}
		out[i].DependsOn = deps
	}
	return out
}

func Unreviewed(st state.Step) bool {
	return st.Status == state.StatusPending || st.Status == state.StatusStale
}
