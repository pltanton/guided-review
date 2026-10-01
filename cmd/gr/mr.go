package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/pltanton/guided-review/internal/diff"
	"github.com/pltanton/guided-review/internal/github"
	"github.com/pltanton/guided-review/internal/gitlab"
	"github.com/pltanton/guided-review/internal/plan"
	"github.com/pltanton/guided-review/internal/state"
)

var verdicts = map[string]string{
	"approve": "approve ✅",
	"changes": "changes requested",
	"blocked": "blocked ⛔",
}

func cmdDiscussions(ctx context.Context, e env, _ []string) error {
	_, r, err := updateReview(ctx, e.dir, func(_ session, r *state.Review) error {
		if r.MR == nil {
			return errors.New("not a merge request review")
		}
		return syncDiscussions(ctx, e, r)
	})
	if err != nil {
		return err
	}
	printDiscussions(e, r, true)
	return nil
}

func cmdPrepare(ctx context.Context, e env, args []string) error {
	fs := e.flags("prepare")
	verdict := fs.String("verdict", "", "approve|changes|blocked")
	decisions := fs.String("decisions", "", "decisions taken during the review and why (markdown)")
	decisionsFile := fs.String("decisions-file", "", "read decisions from a file (- for stdin)")
	approve := fs.Bool("approve", false, "the agent also approves the MR when publishing")
	partial := fs.Bool("partial", false, "allow pending steps: they are listed as not reviewed "+
		"yet and carried into the next round")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *decisionsFile != "" {
		data, err := readInput(e, *decisionsFile)
		if err != nil {
			return err
		}
		*decisions = string(data)
	}
	s, r, err := loadReview(ctx, e.dir)
	if err != nil {
		return err
	}
	switch _, ok := verdicts[*verdict]; {
	case !ok:
		return errors.New("--verdict must be approve, changes or blocked")
	case r.MR == nil && *approve:
		return errors.New("--approve needs a merge request: a local review has nothing to approve")
	case *approve && *verdict != "approve":
		return errors.New("--approve only goes with --verdict approve")
	case r.MR != nil && r.MR.Me == "" && *verdict == "approve":
		return errors.New("approve needs your login on the MR to check your threads: " +
			"run gr discussions")
	case len(r.Steps) == 0:
		return errors.New("no plan yet: nothing reviewed")
	}
	if err := checkNothingUnmarked(s.exportDir(r.ID)); err != nil {
		return err
	}
	if pending := plan.Gate(r); len(pending) > 0 && !*partial {
		return fmt.Errorf("gate not passed, pending: %s (review or skip them first, "+
			"or --partial to send what is reviewed so far)", strings.Join(pending, " "))
	}
	if ids := undecidedThreads(r); len(ids) > 0 {
		return fmt.Errorf("answered threads without the reviewer's decision: %s "+
			"(R in the viewer)", strings.Join(ids, " "))
	}
	if ids := openThreads(r); *verdict == "approve" && len(ids) > 0 {
		return fmt.Errorf("approve with your threads still open: %s "+
			"(resolve them or use --verdict changes)", strings.Join(ids, " "))
	}
	err = s.store.Update(r.ID, func(r *state.Review) error {
		r.Publish = &state.PublishPlan{Verdict: *verdict, Decisions: *decisions, Approve: *approve}
		return nil
	})
	if err != nil {
		return err
	}
	e.println("prepared: the human reviews it and presses P in the viewer to finish")
	return nil
}

type export struct {
	Provider string         `json:"provider"`
	ID       string         `json:"id"`
	HeadSHA  string         `json:"head_sha"`
	Round    int            `json:"round"`
	Request  string         `json:"request,omitempty"`
	Host     string         `json:"host"`
	API      string         `json:"api"`
	URL      string         `json:"url"`
	Verdict  string         `json:"verdict"`
	Approve  bool           `json:"approve"`
	Comments []int          `json:"comments"`
	Summary  bool           `json:"summary"`
	Drafts   []draft        `json:"drafts,omitempty"`
	Review   []sent         `json:"review,omitempty"`
	Threads  []threadAction `json:"threads,omitempty"`
}

const publishedLog = "published.jsonl"

type sent struct {
	Kind string `json:"kind"`
	ID   string `json:"id,omitempty"`
	Part string `json:"part,omitempty"`
}

type draft struct {
	File string `json:"file"`
	sent
}

func sentComment(id int) sent { return sent{Kind: "comment", ID: strconv.Itoa(id)} }

type threadAction struct {
	ID       string `json:"id"`
	Reply    string `json:"reply,omitempty"`
	Resolve  bool   `json:"resolve"`
	ReplyAPI string `json:"reply_api,omitempty"`
	API      string `json:"api,omitempty"`
}

func threadActions(r *state.Review, md *strings.Builder) []threadAction {
	var out []threadAction
	for _, d := range r.MyThreads() {
		t := r.ThreadState(d)
		if !t.Decided() {
			continue
		}
		a := threadAction{ID: d.ID, Reply: t.Reply, Resolve: t.Verdict == state.VerdictResolve}
		if t.ReplyPosted {
			a.Reply = ""
		}
		if a.Reply == "" && !a.Resolve {
			continue
		}
		if r.MR.Provider == state.ProviderGitHub {
			if a.Reply != "" {
				ref := github.PRRef{Project: r.MR.Project, Number: r.MR.IID}
				a.ReplyAPI = ref.Path(fmt.Sprintf("/comments/%d/replies", d.ReplyTo))
			}
		} else {
			ref := gitlab.MRRef{Project: r.MR.Project, IID: r.MR.IID}
			a.API = ref.Path("/discussions/" + d.ID)
			if a.Reply != "" {
				a.ReplyAPI = a.API + "/notes"
			}
		}
		where := "general"
		if d.File != "" {
			where = fmt.Sprintf("%s:%d", d.File, d.Line)
		}
		verdict := "keep open"
		if a.Resolve {
			verdict = "resolve"
		}
		fmt.Fprintf(md, "--- thread %s %s %s\n%s\n\n", verdict, where, d.ID, a.Reply)
		out = append(out, a)
	}
	return out
}

func cmdExport(ctx context.Context, e env, args []string) error {
	fs := e.flags("export")
	dryRun := fs.Bool("dry-run", false, "print review.md and write nothing")
	onlyDir := fs.Bool("dir", false, "print the export directory and write nothing")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if !*onlyDir && !*dryRun {
		_, _, err := updateReview(ctx, e.dir, func(s session, r *state.Review) error {
			return exportReview(ctx, e, s, r, false)
		})
		return err
	}
	s, r, err := loadReview(ctx, e.dir)
	if err != nil {
		return err
	}
	if *onlyDir {
		e.println(s.exportDir(r.ID))
		return nil
	}
	return exportReview(ctx, e, s, r, true)
}

func exportReview(ctx context.Context, e env, s session, r *state.Review, dryRun bool) error {
	dir := s.exportDir(r.ID)
	p := r.Publish
	if p == nil {
		return errors.New("nothing prepared: run gr prepare first")
	}
	if r.MR == nil {
		return exportFixes(e, r, dir, dryRun)
	}
	if !dryRun {
		if err := checkNothingUnmarked(dir); err != nil {
			return err
		}
	}
	mrFiles, err := s.serverDiff(ctx, r.BaseSHA, r.HeadSHA)
	if err != nil {
		return err
	}
	x := export{ID: rand.Text(), HeadSHA: r.HeadSHA, Round: max(r.Round, 1), URL: r.MR.URL,
		Verdict: p.Verdict, Approve: p.Approve && !p.Approved}
	var out exportFiles
	if r.MR.Provider == state.ProviderGitHub {
		out, err = exportGitHub(e, r, mrFiles, &x, dryRun)
	} else {
		out, err = exportGitLab(e, r, mrFiles, &x, dryRun)
	}
	if err != nil || out == nil {
		return err
	}
	if err := out.json("review.json", x); err != nil {
		return err
	}
	if err := writeExport(e, dir, out); err != nil {
		return err
	}
	p.Export = x.ID
	return nil
}

func checkNothingUnmarked(dir string) error {
	if _, err := os.Stat(filepath.Join(dir, publishedLog)); err == nil {
		return errors.New("the last export went out in part and is not marked yet: " +
			"run gr mark-published first")
	}
	return nil
}

func exportGitLab(
	e env,
	r *state.Review,
	mrFiles []diff.File,
	x *export,
	dryRun bool,
) (exportFiles, error) {
	ref := gitlab.MRRef{Host: r.MR.Host, Project: r.MR.Project, IID: r.MR.IID}
	x.Provider, x.Host, x.API = "gitlab", ref.Host, ref.Path("")
	var md strings.Builder
	var notes []gitlab.DraftNote
	var kinds []sent
	for _, c := range r.Comments {
		if c.Published || c.Resolved {
			continue
		}
		note, where := commentDraft(r, c, mrFiles)
		fmt.Fprintf(&md, "--- #%d %s\n%s\n\n", c.ID, where, note.Note)
		notes = append(notes, note)
		kinds = append(kinds, sentComment(c.ID))
		x.Comments = append(x.Comments, c.ID)
	}
	x.Threads = threadActions(r, &md)
	if x.Summary = r.SummaryRound != max(r.Round, 1); x.Summary {
		summary := summaryMarkdown(r, x.Verdict, r.Publish.Decisions)
		fmt.Fprintf(&md, "--- summary\n%s\n", summary)
		notes = append(notes, gitlab.DraftNote{Note: summary})
		kinds = append(kinds, sent{Kind: "summary"})
	}
	if dryRun {
		e.printf("%s", md.String())
		return nil, nil
	}
	out := exportFiles{"review.md": []byte(md.String())}
	for i, n := range notes {
		name := fmt.Sprintf("drafts/%02d.json", i+1)
		if err := out.json(name, n); err != nil {
			return nil, err
		}
		x.Drafts = append(x.Drafts, draft{File: name, sent: kinds[i]})
	}
	return out, nil
}

func cmdMarkPublished(ctx context.Context, e env, _ []string) error {
	var marked, threads int
	var missing []string
	var dir string
	_, _, err := updateReview(ctx, e.dir, func(s session, r *state.Review) error {
		if r.MR == nil {
			return errors.New("a local review is not published: its result is fixes.json")
		}
		dir = s.exportDir(r.ID)
		data, err := os.ReadFile(filepath.Join(dir, "review.json"))
		if err != nil {
			return fmt.Errorf("no export to mark: %w", err)
		}
		var x export
		if err := json.Unmarshal(data, &x); err != nil {
			return err
		}
		p := r.Publish
		switch {
		case p == nil:
			return errors.New("nothing prepared: this export is marked already")
		case x.ID != p.Export || x.HeadSHA != r.HeadSHA || x.Round != max(r.Round, 1):
			return errors.New("the export does not match the review (prepared again or a new " +
				"round): run gr export and publish it again")
		}
		done, err := readPublished(filepath.Join(dir, publishedLog))
		if err != nil {
			return err
		}
		for _, id := range x.Comments {
			if !done[sentComment(id)] {
				missing = append(missing, fmt.Sprintf("#%d", id))
				continue
			}
			if i := slices.IndexFunc(r.Comments, func(c state.Comment) bool { return c.ID == id }); i >= 0 {
				r.Comments[i].Published = true
				marked++
			}
		}
		if x.Summary && done[sent{Kind: "summary"}] {
			r.SummaryRound = max(r.Round, 1)
		} else if x.Summary {
			missing = append(missing, "summary")
		}
		if x.Approve && done[sent{Kind: "approve"}] {
			p.Approved = true
		} else if x.Approve {
			missing = append(missing, "approve")
		}
		if done[sent{Kind: "verdict"}] {
			p.VerdictSent = true
		}
		for _, a := range x.Threads {
			d := r.Discussion(a.ID)
			if d == nil {
				continue
			}
			t := r.ThreadState(*d)
			replied := a.Reply == "" || done[sent{Kind: "thread", ID: a.ID, Part: "reply"}]
			resolved := !a.Resolve || done[sent{Kind: "thread", ID: a.ID, Part: "resolve"}]
			t.ReplyPosted = t.ReplyPosted || a.Reply != "" && replied
			if t.Published = replied && resolved; t.Published {
				threads++
			} else {
				missing = append(missing, "thread "+a.ID)
			}
			r.SetThread(t)
		}
		if len(missing) == 0 {
			r.Publish = nil
		}
		return nil
	})
	if err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(dir, publishedLog)); err != nil && !os.IsNotExist(err) {
		return err
	}
	e.printf("marked %d comments and %d threads published\n", marked, threads)
	if len(missing) > 0 {
		e.printf("not published: %s (run gr export, then publish again: "+
			"it sends only these)\n", strings.Join(missing, ", "))
	}
	return nil
}

func readPublished(path string) (map[sent]bool, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	done := map[sent]bool{}
	for line := range strings.Lines(string(data)) {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var x sent
		if err := json.Unmarshal([]byte(line), &x); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		done[x] = true
	}
	return done, nil
}

func exportGitHub(
	e env,
	r *state.Review,
	mrFiles []diff.File,
	x *export,
	dryRun bool,
) (exportFiles, error) {
	p := r.Publish
	ref := github.PRRef{Host: r.MR.Host, Project: r.MR.Project, Number: r.MR.IID}
	x.Provider, x.Host, x.API = state.ProviderGitHub, ref.Host, ref.Path("/reviews")
	req := github.Review{CommitID: r.HeadSHA, Event: "COMMENT"}
	var md strings.Builder
	var parts, general []string
	for _, c := range r.Comments {
		if c.Published || c.Resolved {
			continue
		}
		x.Comments = append(x.Comments, c.ID)
		x.Review = append(x.Review, sentComment(c.ID))
		body := fmt.Sprintf("**%s** %s", c.Severity, c.Body)
		if c.Suggestion != "" {
			body += fmt.Sprintf("\n\n```suggestion\n%s\n```", c.Suggestion)
		}
		body += "\n\n" + state.CommentMarker(c.ID)
		start, end, _ := state.ParseLines(c.Lines)
		hunk, ok := prHunk(mrFiles, c.File, end)
		if c.SHA != r.HeadSHA || !ok {
			note := fmt.Sprintf("`%s:%s` %s", c.File, c.Lines, body)
			general = append(general, note)
			fmt.Fprintf(&md, "--- #%d %s:%d (general note)\n%s\n\n", c.ID, c.File, end, note)
			continue
		}
		rc := github.ReviewComment{Path: c.File, Line: end, Side: "RIGHT", Body: body}
		switch {
		case start < end && start >= hunk.from:
			rc.StartLine, rc.StartSide = start, "RIGHT"
		case start < end:
			rc.Body = fmt.Sprintf("`%s:%s` **%s** %s\n\n%s",
				c.File, c.Lines, c.Severity, c.Body, state.CommentMarker(c.ID))
			body = rc.Body
		}
		req.Comments = append(req.Comments, rc)
		fmt.Fprintf(&md, "--- #%d %s:%d\n%s\n\n", c.ID, c.File, end, body)
	}
	x.Threads = threadActions(r, &md)
	if x.Summary = r.SummaryRound != max(r.Round, 1); x.Summary {
		summary := summaryMarkdown(r, p.Verdict, p.Decisions)
		parts = append(parts, summary)
		fmt.Fprintf(&md, "--- summary\n%s\n", summary)
		x.Review = append(x.Review, sent{Kind: "summary"})
	}
	req.Body = strings.Join(append(parts, general...), "\n\n")
	switch {
	case p.VerdictSent:
	case p.Verdict == "approve" && x.Approve:
		req.Event = "APPROVE"
		x.Review = append(x.Review, sent{Kind: "approve"})
	case p.Verdict != "approve":
		req.Event = "REQUEST_CHANGES"
	}
	if !p.VerdictSent {
		x.Review = append(x.Review, sent{Kind: "verdict"})
	}
	switch {
	case req.Body != "" || req.Event == "COMMENT":
	case len(req.Comments) > 0:
		req.Body = "See the inline comments."
	case req.Event == "REQUEST_CHANGES":
		req.Body = "Changes still requested."
		if n := len(openThreads(r)); n > 0 {
			req.Body = fmt.Sprintf("Changes still requested: %d open threads.", n)
		}
	}
	if dryRun {
		e.printf("%s", md.String())
		return nil, nil
	}
	out := exportFiles{"review.md": []byte(md.String())}
	if len(x.Review) > 0 {
		x.Request = "review-request.json"
		if err := out.json(x.Request, req); err != nil {
			return nil, err
		}
	}
	return out, nil
}

type lineSpan struct{ from, to int }

// GitHub accepts review comments only on lines its PR diff shows: changes and three lines
// of context around them, in one hunk per comment.
const prContext = 3

func prHunk(files []diff.File, path string, line int) (lineSpan, bool) {
	i := slices.IndexFunc(files, func(f diff.File) bool { return f.Path == path })
	if i < 0 {
		return lineSpan{}, false
	}
	for _, h := range prHunks(files[i]) {
		if line >= h.from && line <= h.to {
			return h, true
		}
	}
	return lineSpan{}, false
}

func prHunks(f diff.File) []lineSpan {
	var out []lineSpan
	for _, h := range f.Hunks {
		from, to := h.NewStart, h.NewStart+h.NewLines-1
		if h.NewLines == 0 {
			from, to = h.NewStart+1, h.NewStart
		}
		span := lineSpan{max(from-prContext, 1), to + prContext}
		if n := len(out); n > 0 && span.from <= out[n-1].to+1 {
			out[n-1].to = span.to
			continue
		}
		out = append(out, span)
	}
	return out
}

const modeSelf = "self"

type fix struct {
	ID         int    `json:"id"`
	Severity   string `json:"severity"`
	File       string `json:"file"`
	Lines      string `json:"lines"`
	Body       string `json:"body"`
	Suggestion string `json:"suggestion,omitempty"`
}

func exportFixes(e env, r *state.Review, dir string, dryRun bool) error {
	p := r.Publish
	x := struct {
		Verdict    string   `json:"verdict"`
		Decisions  string   `json:"decisions"`
		Fixes      []fix    `json:"fixes"`
		Unreviewed []string `json:"unreviewed,omitempty"`
	}{Verdict: p.Verdict, Decisions: p.Decisions}
	for _, st := range r.Steps {
		if st.Status == state.StatusPending {
			x.Unreviewed = append(x.Unreviewed, st.ID+" "+st.Title)
		}
	}
	var md strings.Builder
	for _, c := range r.Comments {
		if c.Resolved {
			continue
		}
		fmt.Fprintf(&md, "--- #%d %s:%s\n**%s** %s\n", c.ID, c.File, c.Lines, c.Severity, c.Body)
		if c.Suggestion != "" {
			fmt.Fprintf(&md, "```suggestion\n%s\n```\n", c.Suggestion)
		}
		md.WriteString("\n")
		x.Fixes = append(x.Fixes, fix{
			ID: c.ID, Severity: string(c.Severity), File: c.File, Lines: c.Lines, Body: c.Body,
			Suggestion: c.Suggestion,
		})
	}
	fmt.Fprintf(&md, "--- summary\n%s\n", summaryMarkdown(r, p.Verdict, p.Decisions))
	if dryRun {
		e.printf("%s", md.String())
		return nil
	}
	out := exportFiles{"review.md": []byte(md.String())}
	if err := out.json("fixes.json", x); err != nil {
		return err
	}
	return writeExport(e, dir, out)
}

type exportFiles map[string][]byte

func (f exportFiles) json(name string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	f[name] = append(data, '\n')
	return nil
}

func writeExport(e env, dir string, files exportFiles) error {
	parent := filepath.Dir(dir)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(parent, "."+filepath.Base(dir)+"-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	for name, data := range files {
		path := filepath.Join(tmp, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			return err
		}
	}
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	if err := os.Rename(tmp, dir); err != nil {
		return err
	}
	e.println(dir)
	return nil
}

func commentDraft(
	r *state.Review,
	c state.Comment,
	mrFiles []diff.File,
) (gitlab.DraftNote, string) {
	start, end, _ := state.ParseLines(c.Lines)
	body := fmt.Sprintf("**%s** %s", c.Severity, c.Body)
	if c.Suggestion != "" {
		body += fmt.Sprintf("\n\n```suggestion:-%d+0\n%s\n```", end-start, c.Suggestion)
	}
	body += "\n\n" + state.CommentMarker(c.ID)
	where := fmt.Sprintf("%s:%d", c.File, end)
	i := slices.IndexFunc(mrFiles, func(f diff.File) bool { return f.Path == c.File })
	if c.SHA != r.HeadSHA || i < 0 {
		note := fmt.Sprintf("`%s:%s` %s", c.File, c.Lines, body)
		return gitlab.DraftNote{Note: note}, where + " (general note)"
	}
	f := mrFiles[i]
	pos := &gitlab.Position{
		PositionType: "text",
		BaseSHA:      r.BaseSHA,
		StartSHA:     r.StartSHA,
		HeadSHA:      r.HeadSHA,
		OldPath:      f.Path,
		NewPath:      f.Path,
		NewLine:      end,
	}
	if f.OldPath != "" {
		pos.OldPath = f.OldPath
	}
	if old, added := f.OldLineFor(end); !added {
		pos.OldLine = old
	}
	return gitlab.DraftNote{Note: body, Position: pos}, where
}

func summaryMarkdown(r *state.Review, verdict, decisions string) string {
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }
	w("**Guided review: %s**", verdicts[verdict])
	if r.Round > 1 {
		w(" (round %d)", r.Round)
	}
	w("\n\n")
	if d := strings.TrimSpace(decisions); d != "" {
		w("%s\n\n", d)
	}
	open := map[state.Severity]int{}
	var resolved []string
	round := max(r.Round, 1)
	for _, c := range r.Comments {
		switch {
		case !c.Resolved:
			open[c.Severity]++
		case c.ResolvedRound == round && (c.Published || r.MR == nil && c.Round < round):
			resolved = append(resolved, fmt.Sprintf("#%d", c.ID))
		}
	}
	var counts []string
	for _, sev := range state.Severities {
		if n := open[sev]; n > 0 {
			counts = append(counts, fmt.Sprintf("%d %s", n, sev))
		}
	}
	if len(counts) > 0 {
		w("Comments: %s, inline.\n", strings.Join(counts, ", "))
	} else {
		w("No comments.\n")
	}
	if len(resolved) > 0 {
		w("Resolved since the last round: %s.\n", strings.Join(resolved, ", "))
	}
	var skipped, pending []string
	for _, st := range r.Steps {
		switch st.Status {
		case state.StatusPending:
			pending = append(pending, st.ID+" "+st.Title)
		case state.StatusSkipped:
			skipped = append(skipped, fmt.Sprintf("%s (%s)", st.Title, st.SkipReason))
		case state.StatusStale:
			skipped = append(skipped, st.Title+" (waits for the blocker fix)")
		}
	}
	if len(skipped) > 0 {
		w("Not reviewed: %s.\n", strings.Join(skipped, "; "))
	}
	if marked := unreviewedMarkerFiles(r); len(marked) > 0 {
		w("Not reviewed, generated by their header comment: %s.\n", strings.Join(marked, ", "))
	}
	if len(pending) > 0 {
		w("Not reviewed yet, comes in the next round: %s.\n", strings.Join(pending, "; "))
	}
	return b.String()
}
