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
	"slices"
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
		var roundFiles []diff.File
		if r.HeadSHA != t.head {
			if roundFiles, err = startRound(ctx, s.repo, r, t); err != nil {
				return err
			}
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
		if roundFiles != nil {
			printRound(e.stdout, r, roundFiles, codeDir(s, r))
			return nil
		}
		fmt.Fprintf(e.stdout, "review %s already exists, resuming (--force to start over)\ncode: %s\n\n", t.id, codeDir(s, r))
		return cmdStatus(ctx, e, nil)
	}
	r, files, err := buildReview(ctx, s.repo, t)
	if err != nil {
		return err
	}
	if old, err := s.store.Load(t.id); err == nil && len(old.Comments) > 0 {
		r.Comments = old.Comments
		fmt.Fprintf(e.stdout, "starting over, kept %d comments\n", len(r.Comments))
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
	cfg, err := config.Load(repo.Dir)
	if err != nil {
		return nil, nil, err
	}
	files, stateFiles, err := reviewFiles(ctx, repo, cfg, t.base, t.head)
	if err != nil {
		return nil, nil, err
	}
	r := &state.Review{ID: t.id, Source: t.source, BaseSHA: t.base, StartSHA: t.start, HeadSHA: t.head, MR: t.mr, Domain: cfg.Domain, Files: stateFiles}
	return r, files, nil
}

func reviewFiles(ctx context.Context, repo gitx.Repo, cfg config.Config, base, head string) ([]diff.File, []state.File, error) {
	files, err := parseDiff(ctx, repo, base, head)
	if err != nil {
		return nil, nil, err
	}
	if len(files) == 0 {
		return nil, nil, errors.New("no changes between base and head")
	}
	attrs, _ := os.ReadFile(filepath.Join(repo.Dir, ".gitattributes"))
	cls := classify.New(classify.DefaultPatterns, cfg.Generated, classify.GitattributesPatterns(string(attrs)))
	var out []state.File
	for _, f := range files {
		tier := state.TierCore
		if cls.Generated(f.Path) || f.Status != diff.Deleted && !f.Binary && hasMarker(ctx, repo, head, f.Path) {
			tier = state.TierGenerated
		}
		added, deleted := f.Stat()
		out = append(out, state.File{Path: f.Path, OldPath: f.OldPath, Status: string(f.Status), Tier: tier, Added: added, Deleted: deleted})
	}
	return files, out, nil
}

func parseDiff(ctx context.Context, repo gitx.Repo, base, head string) ([]diff.File, error) {
	cfg, err := config.Load(repo.Dir)
	if err != nil {
		return nil, err
	}
	raw, err := repo.DiffWith(ctx, cfg.DiffAlgorithm(gitx.DefaultDiffAlgorithm), base, head)
	if err != nil {
		return nil, err
	}
	return diff.Parse(raw)
}

func startRound(ctx context.Context, repo gitx.Repo, r *state.Review, t target) ([]diff.File, error) {
	cfg, err := config.Load(repo.Dir)
	if err != nil {
		return nil, err
	}
	old := r.HeadSHA
	r.Round = max(r.Round, 1) + 1
	r.PrevHeadSHA = old
	r.RoundBaseSHA, r.RoundRebased, r.RoundFiles = "", false, nil
	if repo.IsAncestor(ctx, old, t.head) {
		r.RoundBaseSHA = old
	} else {
		prev, err := parseDiff(ctx, repo, r.BaseSHA, old)
		if err != nil {
			return nil, err
		}
		cur, err := parseDiff(ctx, repo, t.base, t.head)
		if err != nil {
			return nil, err
		}
		r.RoundRebased = true
		r.RoundFiles = changedPatches(prev, cur)
	}
	r.BaseSHA, r.StartSHA, r.HeadSHA = t.base, t.start, t.head
	files, stateFiles, err := reviewFiles(ctx, repo, cfg, r.DiffBase(), r.HeadSHA)
	if err != nil {
		return nil, err
	}
	r.Files = stateFiles
	r.Steps, r.Current, r.Summary = nil, "", ""
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

func printRound(w io.Writer, r *state.Review, files []diff.File, code string) {
	fmt.Fprintf(w, "review %s  %s..%s\ncode: %s\n", r.ID, short(r.BaseSHA), short(r.HeadSHA), code)
	if r.RoundRebased {
		fmt.Fprintf(w, "round %d: rebased, changed files: %s\n", r.Round, strings.Join(r.RoundFiles, " "))
	} else {
		fmt.Fprintf(w, "round %d: fixups since %s\n", r.Round, short(r.PrevHeadSHA))
	}
	var open []state.Comment
	for _, c := range r.Comments {
		if !c.Resolved {
			open = append(open, c)
		}
	}
	fmt.Fprintf(w, "open comments from earlier rounds: %d\n", len(open))
	for _, c := range open {
		fmt.Fprintf(w, "  #%d %s %s:%s  %s\n", c.ID, c.Severity, c.File, c.Lines, c.Body)
	}
	printDiscussions(w, r)
	fmt.Fprintln(w)
	if r.RoundRebased {
		files = slices.DeleteFunc(files, func(f diff.File) bool { return !slices.Contains(r.RoundFiles, f.Path) })
	}
	printHunks(w, r, files)
	fmt.Fprintln(w, "\nnext: check open comments against the new code, then pipe a plan for this round to `gr plan set`")
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

func cmdDiscussions(ctx context.Context, e env) error {
	s, r, err := loadReview(ctx, e.dir)
	if err != nil {
		return err
	}
	if r.MR == nil {
		return errors.New("not a merge request review")
	}
	if err := syncDiscussions(ctx, e.glab, r); err != nil {
		return err
	}
	if err := s.store.Save(r); err != nil {
		return err
	}
	for _, d := range r.Discussions {
		if d.Resolved {
			continue
		}
		where := ""
		if d.File != "" {
			where = fmt.Sprintf(" %s:%d", d.File, d.Line)
		}
		fmt.Fprintf(e.stdout, "@%s%s (%d replies)\n%s\n\n", d.Author, where, d.Replies, indent(strings.TrimSpace(d.Body), "  "))
	}
	return nil
}
