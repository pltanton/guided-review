package plan

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/pltanton/guided-review/internal/state"
)

func AddComment(r *state.Review, c state.Comment) (state.Comment, error) {
	if !slices.Contains(state.Severities, c.Severity) {
		return c, fmt.Errorf("severity %q, want one of %v", c.Severity, state.Severities)
	}
	if strings.TrimSpace(c.Body) == "" {
		return c, errors.New("empty comment")
	}
	if r.File(c.File) == nil {
		return c, fmt.Errorf("%s: not in diff", c.File)
	}
	if c.Lines == "" {
		return c, errors.New("comment needs lines")
	}
	from, to, err := state.ParseLines(c.Lines)
	if err != nil {
		return c, err
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
	for i, h := range r.Hotspots {
		if h.File == c.File && h.Line >= from && h.Line <= to {
			r.Hotspots[i].Checked, r.Hotspots[i].Comment = true, c.ID
		}
	}
	return c, nil
}

func ResolveComment(r *state.Review, id int) error {
	for i := range r.Comments {
		if r.Comments[i].ID == id {
			r.Comments[i].Resolved, r.Comments[i].ResolvedRound = true, max(r.Round, 1)
			return nil
		}
	}
	return fmt.Errorf("no comment #%d", id)
}

func DeleteComment(r *state.Review, id int) error {
	i := slices.IndexFunc(r.Comments, func(c state.Comment) bool { return c.ID == id })
	switch {
	case i < 0:
		return fmt.Errorf("no comment #%d", id)
	case r.Comments[i].Published:
		return fmt.Errorf("comment #%d is already on the MR: delete it there", id)
	}
	r.Comments = slices.Delete(r.Comments, i, i+1)
	return nil
}

func EditComment(r *state.Review, id int, body string, severity state.Severity) error {
	if strings.TrimSpace(body) == "" {
		return errors.New("empty comment")
	}
	if severity != "" && !slices.Contains(state.Severities, severity) {
		return fmt.Errorf("severity %q, want one of %v", severity, state.Severities)
	}
	for i := range r.Comments {
		if r.Comments[i].ID != id {
			continue
		}
		r.Comments[i].Body = body
		if severity != "" {
			r.Comments[i].Severity = severity
		}
		return nil
	}
	return fmt.Errorf("no comment #%d", id)
}

func CheckHotspot(r *state.Review, n int, checked bool) error {
	if n < 1 || n > len(r.Hotspots) {
		return fmt.Errorf("%d hotspots, no #%d", len(r.Hotspots), n)
	}
	r.Hotspots[n-1].Checked = checked
	return nil
}
