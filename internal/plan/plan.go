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
	Briefing    string             `yaml:"briefing,omitempty"`
	Boilerplate []string           `yaml:"boilerplate,omitempty"`
	Files       []File             `yaml:"files,omitempty"`
	Hotspots    []state.Hotspot    `yaml:"hotspots,omitempty"`
	Annotations []state.Annotation `yaml:"annotations,omitempty"`
}

type File struct {
	Path  string `yaml:"path"`
	Order int    `yaml:"order,omitempty"`
	Why   string `yaml:"why,omitempty"`
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

func Validate(p Plan, files []diff.File) []error {
	var errs []error
	fail := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf(format, args...))
	}
	inDiff := map[string]bool{}
	for _, f := range files {
		inDiff[f.Path] = true
	}
	for _, b := range p.Boilerplate {
		if !inDiff[b] {
			fail("boilerplate %s: not in diff", b)
		}
	}
	listed := map[string]bool{}
	for _, f := range p.Files {
		switch {
		case !inDiff[f.Path]:
			fail("file %s: not in diff", f.Path)
		case listed[f.Path]:
			fail("file %s: listed twice", f.Path)
		case f.Order < 0:
			fail("file %s: order must be >= 0", f.Path)
		}
		listed[f.Path] = true
	}
	for _, h := range p.Hotspots {
		switch {
		case !inDiff[h.File]:
			fail("hotspot %s:%d: not in diff", h.File, h.Line)
		case h.Line < 0:
			fail("hotspot %s: line must be >= 0", h.File)
		case !slices.Contains(state.HotspotCategories, h.Cat):
			fail("hotspot %s:%d: category %q, want one of %v", h.File, h.Line, h.Cat,
				state.HotspotCategories)
		case strings.TrimSpace(h.Q) == "":
			fail("hotspot %s:%d has no question", h.File, h.Line)
		}
	}
	for _, a := range p.Annotations {
		if err := checkAnnotation(a, inDiff); err != nil {
			fail("%v", err)
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

// Merge adds a partial plan to the review: later entries at the same place replace earlier ones.
func Merge(r *state.Review, p Plan) {
	if p.Briefing != "" {
		r.Briefing = p.Briefing
	}
	for _, b := range p.Boilerplate {
		if f := r.File(b); f != nil && f.Tier == state.TierCore {
			f.Tier = state.TierBoilerplate
		}
	}
	for _, pf := range p.Files {
		f := r.File(pf.Path)
		if f == nil {
			continue
		}
		f.Read = true
		if pf.Why != "" {
			f.Why = pf.Why
		}
		if pf.Order > 0 {
			f.Order = pf.Order
		}
	}
	for _, h := range p.Hotspots {
		i := slices.IndexFunc(r.Hotspots, func(o state.Hotspot) bool {
			return o.File == h.File && o.Line == h.Line
		})
		if i < 0 {
			r.Hotspots = append(r.Hotspots, h)
			continue
		}
		h.Checked, h.Comment = r.Hotspots[i].Checked, r.Hotspots[i].Comment
		r.Hotspots[i] = h
	}
	for _, a := range p.Annotations {
		i := slices.IndexFunc(r.Annotations, func(o state.Annotation) bool {
			return o.File == a.File && o.Line == a.Line && o.Kind == a.Kind
		})
		if i < 0 {
			r.Annotations = append(r.Annotations, a)
		} else {
			r.Annotations[i] = a
		}
	}
}
