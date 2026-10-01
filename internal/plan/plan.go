package plan

import (
	"bytes"
	"fmt"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/pltanton/guided-review/internal/diff"
	"github.com/pltanton/guided-review/internal/state"
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
	if len(p.Steps) == 0 && len(r.Carried) == 0 {
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
	carried := map[string]bool{}
	for _, s := range r.Carried {
		carried[s.ID] = true
	}
	closed := map[string]bool{}
	for i, s := range p.Steps {
		if s.Intro != "" && i > 0 && p.Steps[i-1].Chapter == s.Chapter {
			fail("step %s: intro belongs on the first step of chapter %q", s.ID, s.Chapter)
		}
		if i > 0 && p.Steps[i-1].Chapter != s.Chapter {
			closed[p.Steps[i-1].Chapter] = true
			if closed[s.Chapter] {
				fail("step %s: chapter %q is split; keep its steps together", s.ID, s.Chapter)
			}
		}
		if s.ID == "" {
			fail("step %q: empty id", s.Title)
			continue
		}
		if ids[s.ID] {
			fail("step %s: duplicate id", s.ID)
		}
		if carried[s.ID] {
			fail("step %s: id taken by a step carried from an earlier round", s.ID)
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
		if n := changedLines(s, files); n > MaxStepLines && s.WhyBig == "" {
			fail(
				"step %s: %d changed lines, over %d: split it by meaning (per function, "+
					"layer or concern); if it truly cannot be split, say why in why_big",
				s.ID,
				n,
				MaxStepLines,
			)
		}
		for _, a := range s.Annotations {
			if err := checkAnnotation(a, inDiff); err != nil {
				fail("step %s: %v", s.ID, err)
			}
		}
		for _, h := range s.Hotspots {
			if _, sure := s.HotspotFile(h); h.Line > 0 && !sure {
				fail("step %s: hotspot at line %d needs file: several files in the step",
					s.ID, h.Line)
			}
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
			case !ids[d] && !carried[d]:
				fail("step %s: depends_on %s: no such step", s.ID, d)
			}
		}
	}
	if c := findCycle(p.Steps); c != nil {
		fail("depends_on cycle: %s", strings.Join(c, " → "))
	}
	for _, f := range files {
		if boilerplate[f.Path] || !r.InRound(f.Path) {
			continue
		}
		if rf := r.File(f.Path); rf != nil && rf.Tier == state.TierGenerated {
			continue
		}
		for _, u := range uncovered(f, slices.Concat(p.Steps, r.Carried)) {
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
	case a.To != 0 && a.To < a.Line:
		return fmt.Errorf("%s: to %d is before the line", where, a.To)
	case !slices.Contains(state.AnnotationKinds, a.Kind):
		return fmt.Errorf("%s: kind %q, want one of %v", where, a.Kind, state.AnnotationKinds)
	case strings.TrimSpace(a.Text) == "":
		return fmt.Errorf("%s: empty text", where)
	}
	return nil
}

// MaxStepLines is the reviewer's own limit on what they take in at once (2026-09-29).
const MaxStepLines = 300

func changedLines(s state.Step, files []diff.File) int {
	n := 0
	for _, sh := range s.Hunks {
		i := slices.IndexFunc(files, func(f diff.File) bool { return f.Path == sh.File })
		if i < 0 {
			continue
		}
		from, to, err := state.ParseLines(sh.Lines)
		if err != nil {
			continue
		}
		for _, h := range files[i].Hunks {
			line := h.NewStart
			for _, l := range h.Lines {
				inRange := from == 0 || from <= line && line <= to+1
				if l.Kind != ' ' && inRange {
					n++
				}
				if l.Kind != '-' {
					line++
				}
			}
		}
	}
	return n
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
	in := func(n, slack int) bool {
		return slices.ContainsFunc(ranges, func(r [2]int) bool { return r[0] <= n && n <= r[1]+slack })
	}
	var out []string
	gaps := func(from, to int) {
		for n := from; n <= to; n++ {
			if in(n, 0) {
				continue
			}
			end := n
			for end < to && !in(end+1, 0) {
				end++
			}
			if end == n {
				out = append(out, fmt.Sprintf("%s:%d", f.Path, n))
			} else {
				out = append(out, fmt.Sprintf("%s:%d-%d", f.Path, n, end))
			}
			n = end
		}
	}
	for _, h := range f.Hunks {
		if f.Status == diff.Deleted {
			gaps(h.OldStart, h.OldStart+h.OldLines-1)
			continue
		}
		if h.NewLines > 0 {
			gaps(h.NewStart, h.NewEnd())
		}
		if h.OldLines > 0 && !in(removalAt(h), 1) {
			out = append(out, fmt.Sprintf("%s:%d(del)", f.Path, h.NewStart))
		}
	}
	return out
}

func removalAt(h diff.Hunk) int {
	if h.NewLines == 0 {
		return h.NewStart + 1
	}
	return h.NewStart
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
	r.Steps = make([]state.Step, len(p.Steps), len(p.Steps)+len(r.Carried))
	for i, s := range p.Steps {
		s.Status = state.StatusPending
		s.MayChange = false
		s.SkipReason, s.Announced = "", false
		for _, a := range s.Annotations {
			if a.Detail != "" {
				d := state.Detail{File: a.File, Line: max(a.To, a.Line), Text: a.Detail}
				s.Details = append(s.Details, d)
			}
		}
		for j, h := range s.Hotspots {
			s.Hotspots[j].File, _ = s.HotspotFile(h)
			if h.Detail != "" && h.Line > 0 {
				d := state.Detail{File: s.Hotspots[j].File, Line: h.Line, Text: h.Detail}
				s.Details = append(s.Details, d)
			}
		}
		r.Steps[i] = s
	}
	for _, s := range r.Carried {
		s.Status, s.MayChange, s.SkipReason, s.Announced = state.StatusPending, false, "", false
		r.Steps = append(r.Steps, s)
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
