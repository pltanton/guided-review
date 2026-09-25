package gitx

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
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
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
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

func (r Repo) MergeBase(ctx context.Context, a, b string) (string, error) {
	out, err := r.Run(ctx, "merge-base", a, b)
	return strings.TrimSpace(out), err
}

func (r Repo) CommonDir(ctx context.Context) (string, error) {
	out, err := r.Run(ctx, "rev-parse", "--path-format=absolute", "--git-common-dir")
	return strings.TrimSpace(out), err
}

func (r Repo) Diff(ctx context.Context, base, head string, paths ...string) (string, error) {
	args := []string{"diff", "--no-color", "--no-ext-diff", "--diff-algorithm=histogram", "-U0", "-M", base, head}
	if len(paths) > 0 {
		args = append(append(args, "--"), paths...)
	}
	return r.Run(ctx, args...)
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
