package gitx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strings"

	"github.com/pltanton/guided-review/internal/diff"
)

type Repo struct {
	Dir string
}

func Open(ctx context.Context, dir string) (Repo, error) {
	out, err := Repo{Dir: dir}.Run(ctx, "rev-parse", "--show-toplevel")
	if err != nil {
		return Repo{}, err
	}
	return Repo{Dir: strings.TrimSpace(out)}, nil
}

func (r Repo) Run(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = r.Dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf(
			"git %s: %w: %s",
			strings.Join(args, " "),
			err,
			strings.TrimSpace(stderr.String()),
		)
	}
	return string(out), nil
}

func (r Repo) Commit(ctx context.Context, rev string) (string, error) {
	out, err := r.Run(ctx, "rev-parse", "--verify", "--quiet", rev+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("unknown revision %q", rev)
	}
	return strings.TrimSpace(out), nil
}

var ErrNoMergeBase = errors.New("no common ancestor")

func (r Repo) MergeBase(ctx context.Context, a, b string) (string, error) {
	out, err := r.Run(ctx, "merge-base", a, b)
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return "", fmt.Errorf("%s and %s: %w", a, b, ErrNoMergeBase)
	}
	return strings.TrimSpace(out), err
}

func (r Repo) CommonDir(ctx context.Context) (string, error) {
	out, err := r.Run(ctx, "rev-parse", "--path-format=absolute", "--git-common-dir")
	return strings.TrimSpace(out), err
}

const DefaultDiffAlgorithm = "histogram"

var DiffAlgorithms = []string{"histogram", "patience", "myers", "minimal"}

func (r Repo) Diff(ctx context.Context, base, head string, paths ...string) (string, error) {
	return r.DiffWith(ctx, DefaultDiffAlgorithm, base, head, paths...)
}

var stableDiff = []string{
	"-c", "core.quotePath=false",
	"-c", "diff.noprefix=false",
	"-c", "diff.mnemonicPrefix=false",
	"-c", "diff.relative=false",
	"-c", "diff.interHunkContext=0",
}

func (r Repo) DiffWith(
	ctx context.Context,
	algo, base, head string,
	paths ...string,
) (string, error) {
	if !slices.Contains(DiffAlgorithms, algo) {
		return "", fmt.Errorf("diff algorithm %q, want one of %v", algo, DiffAlgorithms)
	}
	args := append(slices.Clone(stableDiff),
		"diff",
		"--no-color",
		"--no-ext-diff",
		"--no-textconv",
		"--src-prefix=a/",
		"--dst-prefix=b/",
		"--diff-algorithm="+algo,
		"-U0",
		"-M",
		base,
		head,
	)
	if len(paths) > 0 {
		args = append(append(args, "--"), paths...)
	}
	return r.Run(ctx, args...)
}

type Change struct {
	Status        byte
	Path, OldPath string
}

func (r Repo) Changes(ctx context.Context, base, head string, paths ...string) ([]Change, error) {
	args := append(slices.Clone(stableDiff),
		"diff", "--no-ext-diff", "--name-status", "-z", "-M", base, head)
	if len(paths) > 0 {
		args = append(append(args, "--"), paths...)
	}
	out, err := r.Run(ctx, args...)
	if err != nil {
		return nil, err
	}
	fields := strings.Split(strings.TrimSuffix(out, "\x00"), "\x00")
	var changes []Change
	for i := 0; i < len(fields) && fields[i] != ""; {
		c := Change{Status: fields[i][0]}
		switch {
		case (c.Status == 'R' || c.Status == 'C') && i+2 < len(fields):
			c.OldPath, c.Path = fields[i+1], fields[i+2]
			i += 3
		case i+1 < len(fields):
			c.Path = fields[i+1]
			i += 2
		default:
			return nil, fmt.Errorf("git diff --name-status: truncated entry %q", fields[i])
		}
		changes = append(changes, c)
	}
	return changes, nil
}

func (r Repo) Files(
	ctx context.Context,
	algo, base, head string,
	paths ...string,
) ([]diff.File, error) {
	raw, err := r.DiffWith(ctx, algo, base, head, paths...)
	if err != nil {
		return nil, err
	}
	files, err := diff.Parse(raw)
	if err != nil {
		return nil, err
	}
	changes, err := r.Changes(ctx, base, head, paths...)
	if err != nil {
		return nil, err
	}
	var names []Change
	for _, c := range changes {
		names = append(names, c)
		if c.Status == 'T' {
			// git prints a file<->symlink change as a deletion and an addition (diff.c run_diff)
			names = append(names, c)
		}
	}
	if len(names) != len(files) {
		return nil, fmt.Errorf("git diff %s %s: %d patches for %d changed files",
			base, head, len(files), len(changes))
	}
	for i, n := range names {
		files[i].Path, files[i].OldPath = n.Path, n.OldPath
	}
	return files, nil
}

func (r Repo) Show(ctx context.Context, sha, path string) (string, error) {
	return r.Run(ctx, "show", sha+":"+path)
}

func (r Repo) BranchName(ctx context.Context, rev string) string {
	out, err := r.Run(ctx, "rev-parse", "--abbrev-ref", rev)
	name := strings.TrimSpace(out)
	if err != nil || name == "HEAD" || name == rev && looksLikeSHA(rev) {
		return ""
	}
	return name
}

func looksLikeSHA(s string) bool {
	if len(s) < 7 {
		return false
	}
	for _, c := range s {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return false
		}
	}
	return true
}

func (r Repo) WorktreeAdd(ctx context.Context, path, sha string) error {
	_, err := r.Run(ctx, "worktree", "add", "--detach", path, sha)
	return err
}

func (r Repo) WorktreeCheckout(ctx context.Context, path, sha string) error {
	_, err := Repo{Dir: path}.Run(ctx, "checkout", "-q", "--detach", sha)
	return err
}

func (r Repo) WorktreeRemove(ctx context.Context, path string) error {
	_, err := r.Run(ctx, "worktree", "remove", "--force", path)
	return err
}

func (r Repo) IsAncestor(ctx context.Context, ancestor, rev string) bool {
	_, err := r.Run(ctx, "merge-base", "--is-ancestor", ancestor, rev)
	return err == nil
}
