package state

import (
	"regexp"
	"slices"
	"strings"
)

func (r *Review) MyThreads() []Discussion {
	if r.MR == nil || r.MR.Me == "" {
		return nil
	}
	var answered, silent []Discussion
	for _, d := range r.Discussions {
		if d.Author != r.MR.Me || d.Resolved || !d.Resolvable {
			continue
		}
		if t, ok := r.thread(d); ok && t.Published {
			continue
		}
		if r.Answered(d) {
			answered = append(answered, d)
		} else {
			silent = append(silent, d)
		}
	}
	return append(answered, silent...)
}

func (r *Review) Answered(d Discussion) bool {
	return len(d.Notes) > 0 && d.Notes[len(d.Notes)-1].Author != r.MR.Me
}

func (r *Review) ThreadState(d Discussion) Thread {
	if t, ok := r.thread(d); ok {
		return t
	}
	return Thread{ID: d.ID, Notes: len(d.Notes)}
}

func (r *Review) thread(d Discussion) (Thread, bool) {
	i := slices.IndexFunc(r.Threads, func(t Thread) bool { return t.ID == d.ID })
	if i < 0 || r.Threads[i].Notes != len(d.Notes) {
		return Thread{}, false
	}
	return r.Threads[i], true
}

func (r *Review) SetThread(t Thread) {
	i := slices.IndexFunc(r.Threads, func(x Thread) bool { return x.ID == t.ID })
	if i < 0 {
		r.Threads = append(r.Threads, t)
		return
	}
	r.Threads[i] = t
}

func (r *Review) Discussion(id string) *Discussion {
	i := slices.IndexFunc(r.Discussions, func(d Discussion) bool { return d.ID == id })
	if i < 0 {
		return nil
	}
	return &r.Discussions[i]
}

var severityPrefix = regexp.MustCompile(`^(?:` + "`[^`]+` " + `)?\*\*(\w+)\*\*`)

func ThreadSeverity(d Discussion) Severity {
	if m := severityPrefix.FindStringSubmatch(strings.TrimSpace(d.Body)); m != nil {
		if s := Severity(m[1]); slices.Contains(Severities, s) {
			return s
		}
	}
	return ""
}

func (r *Review) CommentFor(d Discussion) *Comment {
	for i := range r.Comments {
		c := &r.Comments[i]
		if c.Published && c.Body != "" && strings.Contains(d.Body, c.Body) &&
			(d.File == "" || d.File == c.File) {
			return c
		}
	}
	return nil
}
