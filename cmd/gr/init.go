package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/pltanton/guided-review/internal/classify"
	"github.com/pltanton/guided-review/internal/diff"
	"github.com/pltanton/guided-review/internal/github"
	"github.com/pltanton/guided-review/internal/gitlab"
	"github.com/pltanton/guided-review/internal/gitx"
	"github.com/pltanton/guided-review/internal/plan"
	"github.com/pltanton/guided-review/internal/state"
)

type target struct {
	id, source        string
	base, start, head string
	branch            string
	mr                *state.MR
	note              string
}

var unsafeID = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func cmdInit(ctx context.Context, e env, args []string) error {
	fs := e.flags("init")
	base := fs.String("base", "", "base revision (default: merge-base with the default branch)")
	id := fs.String("id", "", "review id (default: derived from the MR or branch)")
	force := fs.Bool("force", false, "start over if the review already exists (comments are kept)")
	self := fs.Bool("self", false, "review your own branch; the result goes back to your agent")
	if err := fs.Parse(args); err != nil {
		return err
	}
	s, err := openSession(ctx, e.dir)
	if err != nil {
		return err
	}
	t, err := resolveTarget(ctx, e, s.repo, fs.Arg(0), *base)
	if err != nil {
		return err
	}
	if t.note != "" {
		e.printf("%s\n\n", t.note)
	}
	switch {
	case *self && t.mr != nil:
		return errors.New("--self reviews a local branch, not an MR URL")
	case *self:
		t.id = "self-" + t.id
	}
	if *id != "" {
		t.id = *id
	}
	old, err := s.store.Load(t.id)
	exists := err == nil
	if err != nil && !errors.Is(err, state.ErrNoReview) && !errors.Is(err, os.ErrNotExist) {
		broken, berr := backupState(s, t.id)
		if berr != nil {
			return fmt.Errorf("state of review %s is unreadable: %w; backup failed: %w", t.id, err, berr)
		}
		if !*force {
			return fmt.Errorf("state of review %s is unreadable: %w\nnothing was changed, "+
				"a copy is in %s; `gr init --force` starts over, comments that can be read are kept",
				t.id, err, broken)
		}
		old, exists = &state.Review{Comments: readableComments(broken)}, true
	}
	var prevWorktree string
	if exists {
		prevWorktree = old.Worktree
	}
	worktree, err := ensureWorktree(ctx, e, s, t, prevWorktree)
	if err != nil {
		return err
	}
	var r *state.Review
	var files []diff.File
	switch {
	case exists && !*force && old.HeadSHA == t.head:
		r = old
	case exists && !*force:
		r = old
		if files, err = startRound(ctx, s, r, t); err != nil {
			return err
		}
	default:
		if r, files, err = newReview(ctx, s, t); err != nil {
			return err
		}
		if exists && len(old.Comments) > 0 {
			r.Comments = old.Comments
			e.printf("starting over, kept %d comments\n", len(r.Comments))
		}
	}
	r.Worktree = worktree
	if *self {
		r.Mode = modeSelf
	}
	if files != nil || *self {
		if err := os.RemoveAll(s.exportDir(r.ID)); err != nil {
			return err
		}
		r.Publish = nil
	}
	if err := syncDiscussions(ctx, e, r); err != nil {
		return err
	}
	if err := s.store.Save(r); err != nil {
		return err
	}
	if err := s.store.SetCurrent(r.ID); err != nil {
		return err
	}
	if files == nil {
		e.printf("review %s already exists, resuming (--force to start over)\n", r.ID)
		if planOutdated(r) {
			e.println("plan outdated: made without chapters and step messages, nothing reviewed " +
				"yet — build a new plan and pipe it to `gr plan set`")
		}
		e.println()
		return cmdStatus(ctx, e, nil)
	}
	printIntro(e, s, r, files)
	return nil
}

func backupState(s session, id string) (string, error) {
	path := filepath.Join(s.store.ReviewDir(id), state.FileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	broken := path + ".broken-" + time.Now().Format("20060102-150405")
	return broken, os.WriteFile(broken, data, 0o600)
}

func readableComments(path string) []state.Comment {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var r struct {
		Comments []state.Comment `yaml:"comments"`
	}
	_ = yaml.Unmarshal(data, &r)
	return r.Comments
}

func resolveTarget(
	ctx context.Context,
	e env,
	repo gitx.Repo,
	arg, base string,
) (target, error) {
	switch {
	case base != "" && (github.IsPRURL(arg) || gitlab.IsMRURL(arg) || strings.Contains(arg, "..")):
		return target{}, errors.New("--base goes with a branch, not with an MR URL or a range")
	case github.IsPRURL(arg):
		return githubTarget(ctx, e, repo, arg)
	case gitlab.IsMRURL(arg):
		ref, err := gitlab.ParseMRURL(arg)
		if err != nil {
			return target{}, err
		}
		mr, err := gitlab.FetchMR(ctx, e.glab, ref)
		if err != nil {
			return gitFallback(ctx, repo, gitRef{
				arg: arg, id: fmt.Sprintf("mr-%d", ref.IID), label: fmt.Sprintf("!%d", ref.IID),
				ref: fmt.Sprintf("merge-requests/%d/head", ref.IID), tool: "glab", base: base,
			}, err)
		}
		refs := mr.DiffRefs
		if err := needCommits(ctx, repo,
			[2]string{refs.BaseSHA, "git fetch origin"},
			[2]string{refs.HeadSHA, fmt.Sprintf("git fetch origin merge-requests/%d/head", ref.IID)},
		); err != nil {
			return target{}, err
		}
		return target{
			id:     fmt.Sprintf("mr-%d", ref.IID),
			source: arg,
			branch: mr.SourceBranch,
			base:   refs.BaseSHA,
			start:  refs.StartSHA,
			head:   refs.HeadSHA,
			mr: &state.MR{
				URL:     mr.WebURL,
				Host:    ref.Host,
				Project: ref.Project,
				IID:     ref.IID,
				Title:   mr.Title,
			},
		}, nil
	case strings.Contains(arg, ".."):
		return rangeTarget(ctx, repo, arg)
	}
	rev := arg
	if rev == "" {
		rev = "HEAD"
	}
	headSHA, err := repo.Commit(ctx, rev)
	if err != nil {
		return target{}, err
	}
	if base == "" {
		if base, err = defaultBase(ctx, repo); err != nil {
			return target{}, err
		}
	}
	baseSHA, err := repo.MergeBase(ctx, base, headSHA)
	if err != nil {
		return target{}, err
	}
	branch := repo.BranchName(ctx, rev)
	id := strings.Trim(unsafeID.ReplaceAllString(branch, "-"), "-")
	if id == "" {
		id = short(headSHA)
	}
	return target{id: id, source: rev, base: baseSHA, head: headSHA, branch: branch}, nil
}

func rangeTarget(ctx context.Context, repo gitx.Repo, arg string) (target, error) {
	a, b, ok := strings.Cut(arg, "...")
	if !ok {
		a, b, _ = strings.Cut(arg, "..")
	}
	tip, err := repo.Commit(ctx, cmp.Or(a, "HEAD"))
	if err != nil {
		return target{}, err
	}
	headSHA, err := repo.Commit(ctx, cmp.Or(b, "HEAD"))
	if err != nil {
		return target{}, err
	}
	baseSHA, err := repo.MergeBase(ctx, tip, headSHA)
	switch {
	case errors.Is(err, gitx.ErrNoMergeBase):
		baseSHA = tip
	case err != nil:
		return target{}, err
	}
	return target{
		id:     short(tip) + "-" + short(headSHA),
		source: arg,
		base:   baseSHA,
		head:   headSHA,
		branch: repo.BranchName(ctx, cmp.Or(b, "HEAD")),
	}, nil
}

func defaultBase(ctx context.Context, repo gitx.Repo) (string, error) {
	for _, c := range []string{"origin/HEAD", "origin/main", "origin/master", "main", "master"} {
		if _, err := repo.Commit(ctx, c); err == nil {
			return c, nil
		}
	}
	return "", errors.New("cannot find the default branch: pass --base")
}

func ensureWorktree(ctx context.Context, e env, s session, t target, prev string) (string, error) {
	path := prev
	if _, err := os.Stat(path); err != nil {
		head, err := s.repo.Commit(ctx, "HEAD")
		if err != nil || head == t.head {
			return "", err
		}
		name := fmt.Sprintf("%s-%s-%s", filepath.Base(s.repo.Dir), s.store.Key, t.id)
		path = filepath.Join(e.cacheDir, "guided-review", name)
	}
	if _, err := os.Stat(path); err != nil {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return "", err
		}
		return path, s.repo.WorktreeAdd(ctx, path, t.head)
	}
	if cur, err := (gitx.Repo{Dir: path}).Commit(ctx, "HEAD"); err == nil && cur == t.head {
		return path, nil
	}
	return path, s.repo.WorktreeCheckout(ctx, path, t.head)
}

func newReview(ctx context.Context, s session, t target) (*state.Review, []diff.File, error) {
	files, stateFiles, err := reviewFiles(ctx, s, t.base, t.head)
	if err != nil {
		return nil, nil, err
	}
	r := &state.Review{
		ID:       t.id,
		Source:   t.source,
		BaseSHA:  t.base,
		StartSHA: t.start,
		HeadSHA:  t.head,
		MR:       t.mr,
		Domain:   s.cfg.Domain,
		Files:    stateFiles,
	}
	return r, files, nil
}

func reviewFiles(
	ctx context.Context,
	s session,
	base, head string,
) ([]diff.File, []state.File, error) {
	files, err := s.diff(ctx, base, head)
	if err != nil {
		return nil, nil, err
	}
	if len(files) == 0 {
		return nil, nil, errors.New("no changes between base and head")
	}
	attrs, _ := os.ReadFile(filepath.Join(s.repo.Dir, ".gitattributes"))
	gitattrs := classify.GitattributesPatterns(string(attrs))
	cls := classify.New(
		classify.Set{Source: "config pattern", Patterns: s.cfg.Generated},
		classify.Set{Source: ".gitattributes", Patterns: gitattrs},
		classify.Set{Source: "default pattern", Patterns: classify.DefaultPatterns},
	)
	out := make([]state.File, 0, len(files))
	for _, f := range files {
		tier, reason := state.TierCore, cls.Match(f.Path)
		if reason == "" && f.Status != diff.Deleted && !f.Binary {
			reason = marker(ctx, s, head, f.Path)
		}
		if reason != "" {
			tier = state.TierGenerated
		}
		added, deleted := f.Stat()
		out = append(out, state.File{
			Path:    f.Path,
			OldPath: f.OldPath,
			Status:  string(f.Status),
			Tier:    tier,
			Reason:  reason,
			Added:   added,
			Deleted: deleted,
		})
	}
	return files, out, nil
}

func marker(ctx context.Context, s session, sha, path string) string {
	content, err := s.repo.Show(ctx, sha, path)
	if err != nil {
		return ""
	}
	return classify.Marker(content)
}

func markerOnly(f state.File) bool {
	return f.Tier == state.TierGenerated && strings.HasPrefix(f.Reason, classify.MarkerReason)
}

func unreviewedMarkerFiles(r *state.Review) []string {
	var out []string
	for _, f := range r.Files {
		inStep := slices.ContainsFunc(r.Steps, func(st state.Step) bool {
			return slices.ContainsFunc(st.Hunks, func(h state.StepHunk) bool { return h.File == f.Path })
		})
		if markerOnly(f) && !inStep {
			out = append(out, f.Path)
		}
	}
	return out
}

func startRound(ctx context.Context, s session, r *state.Review, t target) ([]diff.File, error) {
	old, oldRound := r.HeadSHA, max(r.Round, 1)
	steps := r.Steps
	if len(steps) == 0 {
		steps = r.Carried
	}
	r.Round = oldRound + 1
	r.PrevHeadSHA = old
	r.RoundBaseSHA, r.RoundRebased, r.RoundFiles, r.Carried = "", false, nil, nil
	if t.base == r.BaseSHA && s.repo.IsAncestor(ctx, old, t.head) {
		r.RoundBaseSHA = old
	} else {
		prev, err := s.diff(ctx, r.BaseSHA, old)
		if err != nil {
			return nil, err
		}
		cur, err := s.diff(ctx, t.base, t.head)
		if err != nil {
			return nil, err
		}
		r.RoundRebased, r.RoundFiles = true, changedPatches(prev, cur)
	}
	r.BaseSHA, r.StartSHA, r.HeadSHA = t.base, t.start, t.head
	if err := carrySteps(ctx, s, r, steps, oldRound, old); err != nil {
		return nil, err
	}
	files, stateFiles, err := reviewFiles(ctx, s, r.DiffBase(), r.HeadSHA)
	if err != nil {
		return nil, err
	}
	r.Files = stateFiles
	r.Steps, r.Current, r.Summary, r.Publish = nil, "", "", nil
	r.RoundStart = time.Now()
	files = slices.DeleteFunc(files, func(f diff.File) bool { return !r.InRound(f.Path) })
	return files, nil
}

func carrySteps(
	ctx context.Context,
	s session,
	r *state.Review,
	steps []state.Step,
	oldRound int,
	oldHead string,
) error {
	if !slices.ContainsFunc(steps, plan.Unreviewed) {
		return nil
	}
	since, err := s.diff(ctx, oldHead, r.HeadSHA)
	if err != nil {
		return err
	}
	full, err := s.diff(ctx, r.BaseSHA, r.HeadSHA)
	if err != nil {
		return err
	}
	if r.Carried = plan.Carry(steps, oldRound, since, full); len(r.Carried) == 0 || r.RoundRebased {
		return nil
	}
	r.RoundBaseSHA, r.RoundFiles = "", make([]string, len(since))
	for i, f := range since {
		r.RoundFiles[i] = f.Path
	}
	return nil
}

func changedPatches(prev, cur []diff.File) []string {
	sig := func(f diff.File) string {
		var b strings.Builder
		for _, h := range f.Hunks {
			for _, l := range h.Lines {
				b.WriteByte(l.Kind)
				b.WriteString(l.Text)
				b.WriteByte('\n')
			}
		}
		return b.String()
	}
	before := map[string]string{}
	for _, f := range prev {
		before[f.Path] = sig(f)
	}
	var changed []string
	for _, f := range cur {
		if old, ok := before[f.Path]; !ok || old != sig(f) {
			changed = append(changed, f.Path)
		}
	}
	return changed
}

func printIntro(e env, s session, r *state.Review, files []diff.File) {
	printHeader(e, s, r)
	e.println()
	printHunks(e, r, files)
	next := "pipe a plan (YAML) to `gr plan set`"
	if r.Round > 1 {
		next = "check open comments against the new code, " +
			"then pipe a plan for this round to `gr plan set`"
	}
	if len(r.Carried) > 0 && len(files) == 0 {
		next = "check open comments against the new code, then `echo 'steps: []' | gr plan set` " +
			"to go on with the carried steps"
	}
	e.printf("\nnext: %s\n", next)
}

func printHeader(e env, s session, r *state.Review) {
	e.printf(
		"review %s  %s..%s\ncode: %s\n",
		r.ID,
		short(r.BaseSHA),
		short(r.HeadSHA),
		r.CodeDir(s.repo.Dir),
	)
	if r.MR != nil {
		e.printf("%s %s\n%s\n", r.MR.Label(), r.MR.Title, r.MR.URL)
	}
	if r.Domain != "" {
		e.printf("domain: %s\n", r.Domain)
	}
	if r.Mode == modeSelf {
		e.println("mode: self — the author's own change; the result goes back to their agent")
	}
	if r.Round > 1 {
		if r.RoundRebased {
			e.printf(
				"round %d: the base moved (rebase or merge), changed files: %s\n",
				r.Round,
				strings.Join(r.RoundFiles, " "),
			)
		} else {
			e.printf("round %d: fixups since %s\n", r.Round, short(r.PrevHeadSHA))
		}
		var open []state.Comment
		for _, c := range r.Comments {
			if !c.Resolved && c.Round < r.Round {
				open = append(open, c)
			}
		}
		e.printf("open comments from earlier rounds: %d\n", len(open))
		for _, c := range open {
			e.printf("  #%d %s %s:%s  %s\n", c.ID, c.Severity, c.File, c.Lines, c.Body)
		}
	}
	if len(r.Carried) > 0 {
		ids := make([]string, len(r.Carried))
		for i, st := range r.Carried {
			ids[i] = st.ID
		}
		e.printf("carried, not reviewed in an earlier round: %s (they follow this round's plan)\n",
			strings.Join(ids, " "))
	}
	printDiscussions(e, r, false)
}

func syncDiscussions(ctx context.Context, e env, r *state.Review) error {
	if r.MR == nil {
		return nil
	}
	r.Discussions = r.Discussions[:0]
	if r.MR.Provider == state.ProviderGitHub {
		if r.MR.Me == "" {
			me, err := github.CurrentUser(ctx, e.gh, r.MR.Host)
			if err != nil {
				return fmt.Errorf("who you are on %s: %w", r.MR.Host, err)
			}
			r.MR.Me = me
		}
		ref := github.PRRef{Host: r.MR.Host, Project: r.MR.Project, Number: r.MR.IID}
		ds, err := github.FetchDiscussions(ctx, e.gh, ref)
		for _, d := range ds {
			r.Discussions = append(r.Discussions, state.Discussion{
				ID: d.ID, Author: d.Author, Body: d.Body, Replies: d.Replies, File: d.File,
				Line: d.Line, OldLine: d.OldLine, Resolved: d.Resolved, Resolvable: d.Resolvable,
				Notes: notes(d.Notes), ReplyTo: d.ReplyTo, Comment: marked(d.Notes),
			})
		}
		r.LinkComments()
		return err
	}
	if r.MR.Me == "" {
		me, err := gitlab.CurrentUser(ctx, e.glab, r.MR.Host)
		if err != nil {
			return fmt.Errorf("who you are on %s: %w", r.MR.Host, err)
		}
		r.MR.Me = me
	}
	ref := gitlab.MRRef{Host: r.MR.Host, Project: r.MR.Project, IID: r.MR.IID}
	ds, err := gitlab.FetchDiscussions(ctx, e.glab, ref)
	for _, d := range ds {
		r.Discussions = append(r.Discussions, state.Discussion{
			ID: d.ID, Author: d.Author, Body: d.Body, Replies: d.Replies, File: d.File,
			Line: d.Line, OldLine: d.OldLine, Resolved: d.Resolved, Resolvable: d.Resolvable,
			Notes: notes(d.Notes), Comment: marked(d.Notes),
		})
	}
	r.LinkComments()
	return err
}

func notes[N ~struct{ Author, Body string }](in []N) []state.Note {
	out := make([]state.Note, len(in))
	for i, n := range in {
		out[i] = state.Note(n)
		out[i].Body = state.WithoutMarker(out[i].Body)
	}
	return out
}

func marked[N ~struct{ Author, Body string }](in []N) int {
	if len(in) == 0 {
		return 0
	}
	return state.MarkedComment(state.Note(in[0]).Body)
}

func githubTarget(ctx context.Context, e env, repo gitx.Repo, arg string) (target, error) {
	ref, err := github.ParsePRURL(arg)
	if err != nil {
		return target{}, err
	}
	pr, err := github.FetchPR(ctx, e.gh, ref)
	if err != nil {
		return gitFallback(ctx, repo, gitRef{
			arg: arg, id: fmt.Sprintf("pr-%d", ref.Number), label: fmt.Sprintf("#%d", ref.Number),
			ref: fmt.Sprintf("pull/%d/head", ref.Number), tool: "gh",
		}, err)
	}
	if err := needCommits(ctx, repo,
		[2]string{pr.Base.SHA, "git fetch origin"},
		[2]string{pr.Head.SHA, fmt.Sprintf("git fetch origin pull/%d/head", ref.Number)},
	); err != nil {
		return target{}, err
	}
	base, err := repo.MergeBase(ctx, pr.Base.SHA, pr.Head.SHA)
	if err != nil {
		return target{}, err
	}
	return target{
		id:     fmt.Sprintf("pr-%d", ref.Number),
		source: arg,
		branch: pr.Head.Ref,
		base:   base,
		start:  base,
		head:   pr.Head.SHA,
		mr: &state.MR{
			Provider: state.ProviderGitHub, URL: pr.URL, Host: ref.Host,
			Project: ref.Project, IID: ref.Number, Title: pr.Title,
		},
	}, nil
}

func needCommits(ctx context.Context, repo gitx.Repo, shaFetch ...[2]string) error {
	var missing, fetch []string
	for _, sf := range shaFetch {
		if _, err := repo.Commit(ctx, sf[0]); err != nil {
			missing = append(missing, short(sf[0]))
			fetch = append(fetch, sf[1])
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("commit %s not found locally: run %s",
		strings.Join(missing, ", "), strings.Join(fetch, " && "))
}

func planOutdated(r *state.Review) bool {
	if len(r.Steps) == 0 {
		return false
	}
	for _, st := range r.Steps {
		if st.Status != state.StatusPending || st.Chapter != "" && st.Message != "" {
			return false
		}
	}
	return true
}

type gitRef struct {
	arg, id, label, ref, tool, base string
}

func gitFallback(ctx context.Context, repo gitx.Repo, g gitRef, cause error) (target, error) {
	if _, err := repo.Run(ctx, "fetch", "origin", g.ref); err != nil {
		return target{}, fmt.Errorf("%w\n(also tried git fetch origin %s: %v)", cause, g.ref, err)
	}
	head, err := repo.Commit(ctx, "FETCH_HEAD")
	if err != nil {
		return target{}, err
	}
	base := g.base
	if base == "" {
		if base, err = defaultBase(ctx, repo); err != nil {
			return target{}, err
		}
	}
	baseSHA, err := repo.MergeBase(ctx, base, head)
	if err != nil {
		return target{}, err
	}
	why, _, _ := strings.Cut(cause.Error(), "\n")
	note := fmt.Sprintf("%s is not available (%s).\n"+
		"Reviewing %s from git: %s against %s. Without %s there are no MR title and "+
		"discussions, and the result stays local (review.md) to post by hand.\n"+
		"For the full flow install %s and run `%s auth login`, then gr init again.",
		g.tool, why, g.label, g.ref, base, g.tool, g.tool, g.tool)
	return target{id: g.id, source: g.arg, base: baseSHA, head: head, branch: g.label, note: note}, nil
}
