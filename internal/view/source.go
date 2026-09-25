package view

import (
	"context"
	"fmt"

	"github.com/aplotnikov/guided-review/internal/diff"
	"github.com/aplotnikov/guided-review/internal/gitx"
)

type gitSource struct {
	ctx        context.Context
	repo       gitx.Repo
	base, head string
	diffs      map[string]diff.File
	lines      map[string][]string
}

func newGitSource(ctx context.Context, repo gitx.Repo, base, head string) *gitSource {
	return &gitSource{ctx: ctx, repo: repo, base: base, head: head, lines: map[string][]string{}}
}

func (s *gitSource) FileDiff(path string) (diff.File, error) {
	if s.diffs == nil {
		raw, err := s.repo.Diff(s.ctx, s.base, s.head)
		if err != nil {
			return diff.File{}, err
		}
		files, err := diff.Parse(raw)
		if err != nil {
			return diff.File{}, err
		}
		s.diffs = make(map[string]diff.File, len(files))
		for _, f := range files {
			s.diffs[f.Path] = f
		}
	}
	f, ok := s.diffs[path]
	if !ok {
		return diff.File{}, fmt.Errorf("%s: not in diff", path)
	}
	return f, nil
}

func (s *gitSource) Lines(path string) ([]string, error) {
	if l, ok := s.lines[path]; ok {
		return l, nil
	}
	f, err := s.FileDiff(path)
	if err != nil {
		return nil, err
	}
	var lines []string
	if f.Status != diff.Deleted && !f.Binary {
		content, err := s.repo.Show(s.ctx, s.head, path)
		if err != nil {
			return nil, err
		}
		lines = Highlight(path, expandTabs(content))
	}
	s.lines[path] = lines
	return lines, nil
}
