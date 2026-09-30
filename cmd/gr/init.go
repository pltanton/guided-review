package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/pltanton/guided-review/internal/classify"
	"github.com/pltanton/guided-review/internal/diff"
	"github.com/pltanton/guided-review/internal/github"
	"github.com/pltanton/guided-review/internal/gitlab"
	"github.com/pltanton/guided-review/internal/gitx"
	"github.com/pltanton/guided-review/internal/state"
)

type target struct {
	id, source        string
	base, start, head string
	branch            string
	mr                *state.MR
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
	switch {
	case *self && t.mr != nil:
		return errors.New("--self reviews a local branch, not an MR URL")
	case *self:
		t.id = "self-" + t.id
	}
	if *id != "" {
		t.id = *id
	}
	worktree, err := ensureWorktree(ctx, e, s.repo, t)
	if err != nil {
		return err
	}

	old, err := s.store.Load(t.id)
	exists := err == nil
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
		e.printf("review %s already exists, resuming (--force to start over)\ncode: %s\n",
			r.ID, r.CodeDir(s.repo.Dir))
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

func resolveTarget(
	ctx context.Context,
	e env,
	repo gitx.Repo,
	arg, base string,
) (target, error) {
	switch {
	case github.IsPRURL(arg):
		return githubTarget(ctx, e, repo, arg)
	case gitlab.IsMRURL(arg):
		ref, err := gitlab.ParseMRURL(arg)
		if err != nil {
			return target{}, err
		}
		mr, err := gitlab.FetchMR(ctx, e.glab, ref)
		if err != nil {
			return target{}, err
		}
		refs := mr.DiffRefs
		for _, sha := range []string{refs.BaseSHA, refs.HeadSHA} {
			if _, err := repo.Commit(ctx, sha); err != nil {
				return target{}, fmt.Errorf(
					"commit %s not found locally: run git fetch origin",
					short(sha),
				)
			}
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
		a, b, _ := strings.Cut(arg, "..")
		baseSHA, err := repo.Commit(ctx, a)
		if err != nil {
			return target{}, err
		}
		headSHA, err := repo.Commit(ctx, b)
		if err != nil {
			return target{}, err
		}
		id := short(baseSHA) + "-" + short(headSHA)
		return target{
			id:     id,
			source: arg,
			base:   baseSHA,
			head:   headSHA,
			branch: repo.BranchName(ctx, b),
		}, nil
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

func defaultBase(ctx context.Context, repo gitx.Repo) (string, error) {
	for _, c := range []string{"origin/HEAD", "origin/main", "origin/master", "main", "master"} {
		if _, err := repo.Commit(ctx, c); err == nil {
			return c, nil
		}
	}
	return "", errors.New("cannot find the default branch: pass --base")
}

func ensureWorktree(ctx context.Context, e env, repo gitx.Repo, t target) (string, error) {
	head, err := repo.Commit(ctx, "HEAD")
	if err != nil || head == t.head {
		return "", err
	}
	path := filepath.Join(e.cacheDir, "guided-review", filepath.Base(repo.Dir)+"-"+t.id)
	if _, err := os.Stat(path); err != nil {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return "", err
		}
		return path, repo.WorktreeAdd(ctx, path, t.head)
	}
	if cur, err := (gitx.Repo{Dir: path}).Commit(ctx, "HEAD"); err == nil && cur == t.head {
		return path, nil
	}
	return path, repo.WorktreeCheckout(ctx, path, t.head)
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
	cls := classify.New(classify.DefaultPatterns, s.cfg.Generated, gitattrs)
	out := make([]state.File, 0, len(files))
	for _, f := range files {
		tier := state.TierCore
		if cls.Generated(f.Path) ||
			f.Status != diff.Deleted && !f.Binary && hasMarker(ctx, s, head, f.Path) {
			tier = state.TierGenerated
		}
		added, deleted := f.Stat()
		out = append(out, state.File{
			Path:    f.Path,
			OldPath: f.OldPath,
			Status:  string(f.Status),
			Tier:    tier,
			Added:   added,
			Deleted: deleted,
		})
	}
	return files, out, nil
}

func hasMarker(ctx context.Context, s session, sha, path string) bool {
	content, err := s.repo.Show(ctx, sha, path)
	return err == nil && classify.HasMarker(content)
}

func startRound(ctx context.Context, s session, r *state.Review, t target) ([]diff.File, error) {
	old := r.HeadSHA
	r.Round = max(r.Round, 1) + 1
	r.PrevHeadSHA = old
	r.RoundBaseSHA, r.RoundRebased, r.RoundFiles = "", false, nil
	if s.repo.IsAncestor(ctx, old, t.head) {
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
	files, stateFiles, err := reviewFiles(ctx, s, r.DiffBase(), r.HeadSHA)
	if err != nil {
		return nil, err
	}
	r.Files = stateFiles
	r.Steps, r.Current, r.Summary, r.Publish = nil, "", "", nil
	r.RoundStart = time.Now()
	if r.RoundRebased {
		files = slices.DeleteFunc(files, func(f diff.File) bool {
			return !slices.Contains(r.RoundFiles, f.Path)
		})
	}
	return files, nil
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
	next := "pipe a plan (YAML) to `gr plan set`"
	if r.Round > 1 {
		if r.RoundRebased {
			e.printf(
				"round %d: rebased, changed files: %s\n",
				r.Round,
				strings.Join(r.RoundFiles, " "),
			)
		} else {
			e.printf("round %d: fixups since %s\n", r.Round, short(r.PrevHeadSHA))
		}
		var open []state.Comment
		for _, c := range r.Comments {
			if !c.Resolved {
				open = append(open, c)
			}
		}
		e.printf("open comments from earlier rounds: %d\n", len(open))
		for _, c := range open {
			e.printf("  #%d %s %s:%s  %s\n", c.ID, c.Severity, c.File, c.Lines, c.Body)
		}
		next = "check open comments against the new code, " +
			"then pipe a plan for this round to `gr plan set`"
	}
	printDiscussions(e, r, false)
	e.println()
	printHunks(e, r, files)
	e.printf("\nnext: %s\n", next)
}

func syncDiscussions(ctx context.Context, e env, r *state.Review) error {
	if r.MR == nil {
		return nil
	}
	r.Discussions = r.Discussions[:0]
	if r.MR.Provider == state.ProviderGitHub {
		ref := github.PRRef{Host: r.MR.Host, Project: r.MR.Project, Number: r.MR.IID}
		ds, err := github.FetchDiscussions(ctx, e.gh, ref)
		for _, d := range ds {
			r.Discussions = append(r.Discussions, state.Discussion(d))
		}
		return err
	}
	ref := gitlab.MRRef{Host: r.MR.Host, Project: r.MR.Project, IID: r.MR.IID}
	ds, err := gitlab.FetchDiscussions(ctx, e.glab, ref)
	for _, d := range ds {
		r.Discussions = append(r.Discussions, state.Discussion(d))
	}
	return err
}

func githubTarget(ctx context.Context, e env, repo gitx.Repo, arg string) (target, error) {
	ref, err := github.ParsePRURL(arg)
	if err != nil {
		return target{}, err
	}
	pr, err := github.FetchPR(ctx, e.gh, ref)
	if err != nil {
		return target{}, err
	}
	fetch := map[string]string{
		pr.Base.SHA: "git fetch origin",
		pr.Head.SHA: fmt.Sprintf("git fetch origin pull/%d/head", ref.Number),
	}
	for sha, how := range fetch {
		if _, err := repo.Commit(ctx, sha); err != nil {
			return target{}, fmt.Errorf("commit %s not found locally: run %s", short(sha), how)
		}
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
