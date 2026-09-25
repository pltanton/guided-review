package plan

import (
	"bytes"
	"fmt"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/aplotnikov/guided-review/internal/diff"
	"github.com/aplotnikov/guided-review/internal/state"
)

type Plan struct {
	Summary     string       `yaml:"summary"`
	Boilerplate []string     `yaml:"boilerplate"`
	Steps       []state.Step `yaml:"steps"`
}

func Parse(data []byte) (Plan, error) {
	var p Plan
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&p); err != nil {
		return Plan{}, fmt.Errorf("parse plan: %w", err)
	}
	return p, nil
}

func Validate(p Plan, r *state.Review, files []diff.File) []error {
	var errs []error
	fail := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf(format, args...))
	}
	if len(p.Steps) == 0 {
		fail("plan has no steps")
	}
	inDiff := map[string]bool{}
	for _, f := range files {
		inDiff[f.Path] = true
	}
	boilerplate := map[string]bool{}
	for _, b := range p.Boilerplate {
		if !inDiff[b] {
			fail("boilerplate %s: not in diff", b)
		}
		boilerplate[b] = true
	}
	ids := map[string]bool{}
	for _, s := range p.Steps {
		if s.ID == "" {
			fail("step %q: empty id", s.Title)
			continue
		}
		if ids[s.ID] {
			fail("step %s: duplicate id", s.ID)
		}
		ids[s.ID] = true
		if s.Title == "" {
			fail("step %s: empty title", s.ID)
		}
		if len(s.Hunks) == 0 {
			fail("step %s: no hunks", s.ID)
		}
		for _, h := range s.Hunks {
			if !inDiff[h.File] {
				fail("step %s: %s not in diff", s.ID, h.File)
			}
			if _, _, err := state.ParseLines(h.Lines); err != nil {
				fail("step %s: %v", s.ID, err)
			}
		}
		for _, a := range s.Annotations {
			if err := checkAnnotation(a, inDiff); err != nil {
				fail("step %s: %v", s.ID, err)
			}
		}
		for _, h := range s.Hotspots {
			if !slices.Contains(state.HotspotCategories, h.Cat) {
				fail(
					"step %s: hotspot category %q, want one of %v",
					s.ID,
					h.Cat,
					state.HotspotCategories,
				)
			}
			if strings.TrimSpace(h.Q) == "" {
				fail("step %s: hotspot %s has no question", s.ID, h.Cat)
			}
		}
	}
	for _, s := range p.Steps {
		for _, d := range s.DependsOn {
			switch {
			case d == s.ID:
				fail("step %s: depends on itself", s.ID)
			case !ids[d]:
				fail("step %s: depends_on %s: no such step", s.ID, d)
			}
		}
	}
	if c := findCycle(p.Steps); c != nil {
		fail("depends_on cycle: %s", strings.Join(c, " → "))
	}
	for _, f := range files {
		if boilerplate[f.Path] || r.RoundRebased && !slices.Contains(r.RoundFiles, f.Path) {
			continue
		}
		if rf := r.File(f.Path); rf != nil && rf.Tier == state.TierGenerated {
			continue
		}
		for _, u := range uncovered(f, p.Steps) {
			fail("not covered: %s", u)
		}
	}
	return errs
}

func checkAnnotation(a state.Annotation, inDiff map[string]bool) error {
	where := fmt.Sprintf("annotation %s:%d", a.File, a.Line)
	switch {
	case !inDiff[a.File]:
		return fmt.Errorf("%s: not in diff", where)
	case a.Line < 1:
		return fmt.Errorf("%s: line must be >= 1", where)
	case !slices.Contains(state.AnnotationKinds, a.Kind):
		return fmt.Errorf("%s: kind %q, want one of %v", where, a.Kind, state.AnnotationKinds)
	case strings.TrimSpace(a.Text) == "":
		return fmt.Errorf("%s: empty text", where)
	}
	return nil
}

func uncovered(f diff.File, steps []state.Step) []string {
	var ranges [][2]int
	mentioned := false
	for _, s := range steps {
		for _, h := range s.Hunks {
			if h.File != f.Path {
				continue
			}
			mentioned = true
			if h.Lines == "" {
				return nil
			}
			if a, b, err := state.ParseLines(h.Lines); err == nil {
				ranges = append(ranges, [2]int{a, b})
			}
		}
	}
	if len(f.Hunks) == 0 {
		if mentioned {
			return nil
		}
		return []string{f.Path}
	}
	var out []string
	for _, h := range f.Hunks {
		covered := slices.ContainsFunc(ranges, func(r [2]int) bool {
			return r[0] <= h.NewEnd() && h.NewStart <= r[1]
		})
		if !covered {
			out = append(out, fmt.Sprintf("%s:%s", f.Path, h.Range()))
		}
	}
	return out
}

func findCycle(steps []state.Step) []string {
	deps := map[string][]string{}
	for _, s := range steps {
		deps[s.ID] = s.DependsOn
	}
	const (
		unvisited = iota
		inProgress
		finished
	)
	mark := map[string]int{}
	var stack []string
	var visit func(id string) []string
	visit = func(id string) []string {
		mark[id] = inProgress
		stack = append(stack, id)
		for _, d := range deps[id] {
			if _, known := deps[d]; !known || d == id {
				continue
			}
			switch mark[d] {
			case inProgress:
				i := slices.Index(stack, d)
				return append(slices.Clone(stack[i:]), d)
			case unvisited:
				if c := visit(d); c != nil {
					return c
				}
			}
		}
		stack = stack[:len(stack)-1]
		mark[id] = finished
		return nil
	}
	for _, s := range steps {
		if mark[s.ID] == unvisited {
			if c := visit(s.ID); c != nil {
				return c
			}
		}
	}
	return nil
}

func Apply(r *state.Review, p Plan) {
	r.Summary = p.Summary
	r.Steps = make([]state.Step, len(p.Steps))
	for i, s := range p.Steps {
		s.Status = state.StatusPending
		s.MayChange = false
		s.SkipReason = ""
		r.Steps[i] = s
	}
	boilerplate := map[string]bool{}
	for _, b := range p.Boilerplate {
		boilerplate[b] = true
	}
	for i := range r.Files {
		f := &r.Files[i]
		switch {
		case f.Tier == state.TierGenerated:
		case boilerplate[f.Path]:
			f.Tier = state.TierBoilerplate
		default:
			f.Tier = state.TierCore
		}
	}
	r.Current = r.Steps[0].ID
}
