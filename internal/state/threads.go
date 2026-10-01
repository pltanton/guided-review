package state

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
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

var commentMarker = regexp.MustCompile(`<!-- gr:comment (\d+) -->`)

func CommentMarker(id int) string {
	return fmt.Sprintf("<!-- gr:comment %d -->", id)
}

func MarkedComment(body string) int {
	m := commentMarker.FindStringSubmatch(body)
	if m == nil {
		return 0
	}
	id, _ := strconv.Atoi(m[1])
	return id
}

func WithoutMarker(body string) string {
	return strings.TrimSpace(commentMarker.ReplaceAllString(body, ""))
}

func (r *Review) CommentFor(d Discussion) *Comment {
	if i := slices.IndexFunc(r.Comments, func(c Comment) bool { return c.ThreadID == d.ID }); i >= 0 {
		return &r.Comments[i]
	}
	if r.MR == nil || d.Author != r.MR.Me {
		return nil
	}
	if d.Comment > 0 {
		i := slices.IndexFunc(r.Comments, func(c Comment) bool {
			return c.ID == d.Comment && c.Published && c.ThreadID == ""
		})
		if i < 0 {
			return nil
		}
		return &r.Comments[i]
	}
	text := strings.TrimSpace(severityPrefix.ReplaceAllString(d.Body, ""))
	text, _, _ = strings.Cut(text, "\n\n```suggestion")
	var found *Comment
	for i := range r.Comments {
		c := &r.Comments[i]
		if !c.Published || c.ThreadID != "" || strings.TrimSpace(c.Body) != text ||
			d.File != "" && d.File != c.File {
			continue
		}
		if found != nil {
			return nil
		}
		found = c
	}
	return found
}

func (r *Review) LinkComments() {
	for _, d := range r.Discussions {
		if c := r.CommentFor(d); c != nil {
			c.ThreadID = d.ID
		}
	}
}

func (t Thread) Decided() bool {
	return t.Verdict != "" && t.DecidedBy == DecidedByViewer && !t.DecidedAt.IsZero()
}

func (r *Review) Decide(id, verdict, reply string, now time.Time) error {
	d := r.Discussion(id)
	if d == nil {
		return fmt.Errorf("no thread %q", id)
	}
	t := r.ThreadState(*d)
	if i := slices.IndexFunc(r.Comments, func(c Comment) bool { return c.ID == t.Resolves }); i >= 0 {
		r.Comments[i].Resolved = false
	}
	t.Resolves = 0
	switch verdict {
	case VerdictResolve, VerdictOpen:
		t.Verdict, t.Reply = verdict, strings.TrimSpace(reply)
		t.DecidedAt, t.DecidedBy = now, DecidedByViewer
	case VerdictNone:
		t.Verdict, t.Reply, t.DecidedAt, t.DecidedBy = "", "", time.Time{}, ""
	default:
		return fmt.Errorf("verdict %q: want resolve, open or none", verdict)
	}
	if c := r.CommentFor(*d); c != nil && verdict == VerdictResolve && !c.Resolved {
		c.Resolved, t.Resolves = true, c.ID
	}
	r.SetThread(t)
	return nil
}
