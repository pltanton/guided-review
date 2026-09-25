package view

import (
	"context"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/aplotnikov/guided-review/internal/gitx"
	"github.com/aplotnikov/guided-review/internal/testrepo"
)

func TestGitSource(t *testing.T) {
	tr := testrepo.New(t)
	tr.Write("a.go", "package a\n")
	tr.Write("gone.go", "package a\n")
	base := tr.Commit("base")
	tr.Write("a.go", "package a\n\nvar X = 1\n")
	tr.Git("rm", "-q", "gone.go")
	head := tr.Commit("head")

	src := newGitSource(
		context.Background(),
		gitx.Repo{Dir: tr.Dir},
		gitx.DefaultDiffAlgorithm,
		base,
		head,
	)
	lines, err := src.Lines("a.go")
	if err != nil || len(lines) != 3 || ansi.Strip(lines[2]) != "var X = 1" {
		t.Fatalf("Lines(a.go) = %q, %v", lines, err)
	}
	if lines, err := src.Lines("gone.go"); err != nil || lines != nil {
		t.Fatalf("Lines(gone.go) = %q, %v", lines, err)
	}
	if _, err := src.FileDiff("nope.go"); err == nil {
		t.Fatal("FileDiff(nope.go): want error")
	}
}
