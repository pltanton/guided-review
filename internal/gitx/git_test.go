package gitx_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pltanton/guided-review/internal/gitx"
	"github.com/pltanton/guided-review/internal/testrepo"
)

func TestRepo(t *testing.T) {
	ctx := context.Background()
	tr := testrepo.New(t)
	tr.Write("a.txt", "one\n")
	base := tr.Commit("base")
	tr.Git("checkout", "-q", "-b", "feature")
	tr.Write("a.txt", "one\ntwo\n")
	head := tr.Commit("head")

	repo, err := gitx.Open(ctx, tr.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := repo.Commit(ctx, "HEAD"); err != nil || got != head {
		t.Fatalf("Commit(HEAD) = %q, %v; want %q", got, err, head)
	}
	if _, err := repo.Commit(ctx, "nope"); err == nil {
		t.Fatal("Commit(nope): want error")
	}
	if got, err := repo.MergeBase(ctx, "main", "feature"); err != nil || got != base {
		t.Fatalf("MergeBase = %q, %v; want %q", got, err, base)
	}
	d, err := repo.Diff(ctx, base, head)
	if err != nil || !strings.Contains(d, "@@ -1,0 +2 @@") {
		t.Fatalf("Diff = %q, %v", d, err)
	}
	if got, err := repo.Show(ctx, head, "a.txt"); err != nil || got != "one\ntwo\n" {
		t.Fatalf("Show = %q, %v", got, err)
	}
	if got, err := repo.CommonDir(ctx); err != nil || filepath.Base(got) != ".git" {
		t.Fatalf("CommonDir = %q, %v", got, err)
	}
	if got := repo.BranchName(ctx, "HEAD"); got != "feature" {
		t.Fatalf("BranchName(HEAD) = %q", got)
	}
	if got := repo.BranchName(ctx, base); got != "" {
		t.Fatalf("BranchName(sha) = %q, want empty", got)
	}
}

func TestWorktree(t *testing.T) {
	ctx := context.Background()
	tr := testrepo.New(t)
	tr.Write("a.txt", "one\n")
	first := tr.Commit("first")
	tr.Write("a.txt", "two\n")
	second := tr.Commit("second")
	repo := gitx.Repo{Dir: tr.Dir}

	wt := filepath.Join(t.TempDir(), "wt")
	if err := repo.WorktreeAdd(ctx, wt, first); err != nil {
		t.Fatal(err)
	}
	if got, _ := (gitx.Repo{Dir: wt}).Commit(ctx, "HEAD"); got != first {
		t.Fatalf("worktree HEAD = %q, want %q", got, first)
	}
	if err := repo.WorktreeCheckout(ctx, wt, second); err != nil {
		t.Fatal(err)
	}
	if got, _ := (gitx.Repo{Dir: wt}).Commit(ctx, "HEAD"); got != second {
		t.Fatalf("after checkout HEAD = %q, want %q", got, second)
	}
	if err := repo.WorktreeRemove(ctx, wt); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Fatalf("worktree still exists: %v", err)
	}
}

func TestDiffAlgorithm(t *testing.T) {
	ctx := context.Background()
	tr := testrepo.New(t)
	tr.Write("a.txt", "one\n")
	base := tr.Commit("base")
	tr.Write("a.txt", "one\ntwo\n")
	head := tr.Commit("head")
	repo := gitx.Repo{Dir: tr.Dir}
	for _, algo := range gitx.DiffAlgorithms {
		if d, err := repo.DiffWith(ctx, algo, base, head); err != nil ||
			!strings.Contains(d, "+two") {
			t.Fatalf("%s: %q, %v", algo, d, err)
		}
	}
	if _, err := repo.DiffWith(ctx, "bogus", base, head); err == nil {
		t.Fatal("unknown algorithm must fail")
	}
}
