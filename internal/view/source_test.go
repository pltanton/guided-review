package view

import (
	"context"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/pltanton/guided-review/internal/gitx"
	"github.com/pltanton/guided-review/internal/testrepo"
)

func TestGitSource(t *testing.T) {
	tr := testrepo.New(t)
	tr.Write("a.go", "package a\n")
	tr.Write("gone.go", "package a\n")
	tr.Write("same.go", "package a\n\nvar Y = 2\n")
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
	if fd, err := src.FileDiff("same.go"); err != nil || len(fd.Hunks) != 0 {
		t.Fatalf("FileDiff(same.go) = %+v, %v; want no hunks", fd, err)
	}
	if lines, err := src.Lines("same.go"); err != nil || len(lines) != 3 {
		t.Fatalf("an unchanged file still shows its code: %q, %v", lines, err)
	}
}
