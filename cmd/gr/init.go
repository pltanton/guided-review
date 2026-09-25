package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/aplotnikov/guided-review/internal/classify"
	"github.com/aplotnikov/guided-review/internal/config"
	"github.com/aplotnikov/guided-review/internal/diff"
	"github.com/aplotnikov/guided-review/internal/gitlab"
	"github.com/aplotnikov/guided-review/internal/gitx"
	"github.com/aplotnikov/guided-review/internal/plan"
	"github.com/aplotnikov/guided-review/internal/state"
)

type target struct {
	id, source        string
	base, start, head string
	branch            string
	mr                *state.MR
	ref               *gitlab.MRRef
}

func cmdInit(ctx context.Context, e env, args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(e.stdout)
	base := fs.String("base", "", "base revision (default: merge-base with the default branch)")
	id := fs.String("id", "", "review id (default: derived from the MR or branch)")
	force := fs.Bool("force", false, "start over if the review already exists")
	if err := fs.Parse(args); err != nil {
		return err
	}
	s, err := openSession(ctx, e.dir)
	if err != nil {
		return err
	}
	t, err := resolveTarget(ctx, s.repo, e.glab, fs.Arg(0), *base)
	if err != nil {
		return err
	}
	if *id != "" {
		t.id = *id
	}
	head, err := s.repo.Commit(ctx, "HEAD")
	if err != nil {
		return err
	}
	var worktree string
	if head != t.head {
		worktree = filepath.Join(e.cacheDir, "guided-review", filepath.Base(s.repo.Dir)+"-"+t.id)
		if err := ensureWorktree(ctx, s.repo, worktree, t.head); err != nil {
			return err
		}
	}
	if s.store.Exists(t.id) && !*force {
		r, err := s.store.Load(t.id)
		if err != nil {
			return err
		}
		r.Worktree = worktree
		if err := syncDiscussions(ctx, e.glab, r); err != nil {
			return err
		}
		if err := s.store.Save(r); err != nil {
			return err
		}
		if err := s.store.SetCurrent(t.id); err != nil {
			return err
		}
		fmt.Fprintf(e.stdout, "review %s already exists, resuming (--force to start over)\ncode: %s\n\n", t.id, codeDir(s, r))
		return cmdStatus(ctx, e, nil)
	}
	r, files, err := buildReview(ctx, s.repo, t)
	if err != nil {
		return err
	}
	r.Worktree = worktree
	if err := syncDiscussions(ctx, e.glab, r); err != nil {
		return err
	}
	if err := s.store.Save(r); err != nil {
		return err
	}
	if err := s.store.SetCurrent(r.ID); err != nil {
		return err
	}
	printInit(e.stdout, r, files, codeDir(s, r))
	return nil
}

func ensureWorktree(ctx context.Context, repo gitx.Repo, path, sha string) error {
	if _, err := os.Stat(path); err != nil {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		return repo.WorktreeAdd(ctx, path, sha)
	}
	if cur, err := (gitx.Repo{Dir: path}).Commit(ctx, "HEAD"); err == nil && cur == sha {
		return nil
	}
	return repo.WorktreeCheckout(ctx, path, sha)
}

func codeDir(s session, r *state.Review) string {
	if r.Worktree != "" {
		return r.Worktree
	}
	return s.repo.Dir
}

func cmdList(ctx context.Context, e env) error {
	s, err := openSession(ctx, e.dir)
	if err != nil {
		return err
	}
	ids, err := s.store.List()
	if err != nil {
		return err
	}
	current, _ := s.store.Current()
	for _, id := range ids {
		r, err := s.store.Load(id)
		if err != nil {
			return err
		}
		marker := " "
		if id == current {
			marker = "*"
		}
		cov := plan.CoverageOf(r)
		title := r.Source
		if r.MR != nil {
			title = r.MR.Title
		}
		fmt.Fprintf(e.stdout, "%s %s  %d/%d steps  %s\n", marker, id, cov.Done+cov.Skipped, cov.Total, title)
	}
	return nil
}

func cmdDone(ctx context.Context, e env) error {
	s, r, err := loadReview(ctx, e.dir)
	if err != nil {
		return err
	}
	if r.Worktree != "" {
		if err := s.repo.WorktreeRemove(ctx, r.Worktree); err != nil {
			return err
		}
		r.Worktree = ""
		if err := s.store.Save(r); err != nil {
			return err
		}
	}
	if err := s.store.ClearCurrent(); err != nil {
		return err
	}
	fmt.Fprintf(e.stdout, "review %s closed; state kept for a re-review\n", r.ID)
	return nil
}

func cmdHunks(ctx context.Context, e env) error {
	s, r, err := loadReview(ctx, e.dir)
	if err != nil {
		return err
	}
	files, err := s.diffFiles(ctx, r)
	if err != nil {
		return err
	}
	printHunks(e.stdout, r, files)
	return nil
}

func resolveTarget(ctx context.Context, repo gitx.Repo, glab gitlab.Runner, arg, base string) (target, error) {
	switch {
	case gitlab.IsMRURL(arg):
		ref, err := gitlab.ParseMRURL(arg)
		if err != nil {
			return target{}, err
		}
		mr, err := gitlab.FetchMR(ctx, glab, ref)
		if err != nil {
			return target{}, err
		}
		for _, sha := range []string{mr.DiffRefs.BaseSHA, mr.DiffRefs.HeadSHA} {
			if _, err := repo.Commit(ctx, sha); err != nil {
				return target{}, fmt.Errorf("commit %s not found locally: run git fetch origin", short(sha))
			}
		}
		return target{
			id: fmt.Sprintf("mr-%d", ref.IID), source: arg, branch: mr.SourceBranch,
			base: mr.DiffRefs.BaseSHA, start: mr.DiffRefs.StartSHA, head: mr.DiffRefs.HeadSHA,
			mr:  &state.MR{URL: mr.WebURL, Host: ref.Host, Project: ref.Project, IID: ref.IID, Title: mr.Title},
			ref: &ref,
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
		return target{id: short(baseSHA) + "-" + short(headSHA), source: arg, base: baseSHA, head: headSHA, branch: repo.BranchName(ctx, b)}, nil
	default:
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
		id := sanitizeID(branch)
		if id == "" {
			id = short(headSHA)
		}
		return target{id: id, source: rev, base: baseSHA, head: headSHA, branch: branch}, nil
	}
}

func defaultBase(ctx context.Context, repo gitx.Repo) (string, error) {
	for _, c := range []string{"origin/HEAD", "origin/main", "origin/master", "main", "master"} {
		if _, err := repo.Commit(ctx, c); err == nil {
			return c, nil
		}
	}
	return "", errors.New("cannot find the default branch: pass --base")
}

var unsafeID = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func sanitizeID(s string) string {
	return strings.Trim(unsafeID.ReplaceAllString(s, "-"), "-")
}

func buildReview(ctx context.Context, repo gitx.Repo, t target) (*state.Review, []diff.File, error) {
	raw, err := repo.Diff(ctx, t.base, t.head)
	if err != nil {
		return nil, nil, err
	}
	files, err := diff.Parse(raw)
	if err != nil {
		return nil, nil, err
	}
	if len(files) == 0 {
		return nil, nil, errors.New("no changes between base and head")
	}
	cfg, err := config.Load(repo.Dir)
	if err != nil {
		return nil, nil, err
	}
	attrs, _ := os.ReadFile(filepath.Join(repo.Dir, ".gitattributes"))
	cls := classify.New(classify.DefaultPatterns, cfg.Generated, classify.GitattributesPatterns(string(attrs)))

	r := &state.Review{ID: t.id, Source: t.source, BaseSHA: t.base, StartSHA: t.start, HeadSHA: t.head, MR: t.mr, Domain: cfg.Domain}
	for _, f := range files {
		tier := state.TierCore
		if cls.Generated(f.Path) || f.Status != diff.Deleted && !f.Binary && hasMarker(ctx, repo, t.head, f.Path) {
			tier = state.TierGenerated
		}
		added, deleted := f.Stat()
		r.Files = append(r.Files, state.File{Path: f.Path, OldPath: f.OldPath, Status: string(f.Status), Tier: tier, Added: added, Deleted: deleted})
	}
	return r, files, nil
}

func hasMarker(ctx context.Context, repo gitx.Repo, sha, path string) bool {
	content, err := repo.Show(ctx, sha, path)
	return err == nil && classify.HasMarker(content)
}

func printInit(w io.Writer, r *state.Review, files []diff.File, code string) {
	fmt.Fprintf(w, "review %s  %s..%s\ncode: %s\n", r.ID, short(r.BaseSHA), short(r.HeadSHA), code)
	if r.MR != nil {
		fmt.Fprintf(w, "MR !%d %s\n%s\n", r.MR.IID, r.MR.Title, r.MR.URL)
	}
	if r.Domain != "" {
		fmt.Fprintf(w, "domain: %s\n", r.Domain)
	}
	printDiscussions(w, r)
	fmt.Fprintln(w)
	printHunks(w, r, files)
	fmt.Fprintln(w, "\nnext: pipe a plan (YAML) to `gr plan set`")
}

func syncDiscussions(ctx context.Context, glab gitlab.Runner, r *state.Review) error {
	if r.MR == nil {
		return nil
	}
	ds, err := gitlab.FetchDiscussions(ctx, glab, gitlab.MRRef{Host: r.MR.Host, Project: r.MR.Project, IID: r.MR.IID})
	if err != nil {
		return err
	}
	r.Discussions = r.Discussions[:0]
	for _, d := range ds {
		r.Discussions = append(r.Discussions, state.Discussion(d))
	}
	return nil
}

func cmdSync(ctx context.Context, e env) error {
	s, r, err := loadReview(ctx, e.dir)
	if err != nil {
		return err
	}
	if r.MR == nil {
		return errors.New("not a merge request review: nothing to sync")
	}
	if err := syncDiscussions(ctx, e.glab, r); err != nil {
		return err
	}
	if err := s.store.Save(r); err != nil {
		return err
	}
	printDiscussions(e.stdout, r)
	return nil
}

func printDiscussions(w io.Writer, r *state.Review) {
	if r.MR == nil {
		return
	}
	var open []state.Discussion
	for _, d := range r.Discussions {
		if !d.Resolved {
			open = append(open, d)
		}
	}
	fmt.Fprintf(w, "MR discussions: %d unresolved\n", len(open))
	for _, d := range open {
		where := ""
		if d.File != "" {
			where = fmt.Sprintf(" %s:%d", d.File, d.Line)
		}
		body, _, _ := strings.Cut(d.Body, "\n")
		if len(body) > 120 {
			body = body[:117] + "..."
		}
		fmt.Fprintf(w, "  @%s%s: %s\n", d.Author, where, body)
	}
}
