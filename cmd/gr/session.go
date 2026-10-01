package main

import (
	"cmp"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/pltanton/guided-review/internal/config"
	"github.com/pltanton/guided-review/internal/diff"
	"github.com/pltanton/guided-review/internal/gitx"
	"github.com/pltanton/guided-review/internal/state"
)

type session struct {
	repo  gitx.Repo
	store state.Store
	cfg   config.Config
}

func openSession(ctx context.Context, dir string) (session, error) {
	repo, err := gitx.Open(ctx, dir)
	if err != nil {
		return session{}, err
	}
	common, err := repo.CommonDir(ctx)
	if err != nil {
		return session{}, err
	}
	cfg, err := config.Load(repo.Dir)
	if err != nil {
		return session{}, err
	}
	key := sha1.Sum([]byte(repo.Dir))
	store := state.Store{
		Dir: filepath.Join(common, "guided-review"),
		Key: hex.EncodeToString(key[:])[:12],
	}
	return session{repo: repo, store: store, cfg: cfg}, nil
}

func loadReview(ctx context.Context, dir string) (session, *state.Review, error) {
	s, err := openSession(ctx, dir)
	if err != nil {
		return session{}, nil, err
	}
	r, err := s.store.LoadCurrent()
	return s, r, err
}

func (s session) exportDir(id string) string {
	return filepath.Join(s.store.Dir, "exports", id)
}

func (s session) exported(id string) string {
	for _, name := range []string{"fixes.json", "review.json"} {
		path := filepath.Join(s.exportDir(id), name)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

func (s session) diff(ctx context.Context, base, head string) ([]diff.File, error) {
	algo := cmp.Or(s.cfg.Diff, gitx.DefaultDiffAlgorithm)
	return s.repo.Files(ctx, algo, base, head)
}

func (s session) reviewDiff(ctx context.Context, r *state.Review) ([]diff.File, error) {
	return s.diff(ctx, r.DiffBase(), r.HeadSHA)
}

func short(sha string) string {
	return sha[:min(len(sha), 8)]
}

func indent(s, prefix string) string {
	return prefix + strings.ReplaceAll(s, "\n", "\n"+prefix)
}

func parseID(s string) (int, error) {
	id, err := strconv.Atoi(strings.TrimPrefix(s, "#"))
	if err != nil {
		return 0, fmt.Errorf("comment id %q: %w", s, err)
	}
	return id, nil
}

func printHunks(e env, r *state.Review, files []diff.File) {
	var generated []state.File
	e.println("hunks (new-file line ranges):")
	for _, f := range files {
		rf := r.File(f.Path)
		switch {
		case rf == nil:
			continue
		case rf.Tier == state.TierGenerated:
			generated = append(generated, *rf)
			continue
		}
		label := string(f.Status)
		if f.OldPath != "" {
			label += " from " + f.OldPath
		}
		if f.Binary {
			label += ", binary"
		}
		if rf.Tier == state.TierBoilerplate {
			label += ", boilerplate"
		}
		ranges := make([]string, len(f.Hunks))
		for i, h := range f.Hunks {
			ranges[i] = h.Range()
		}
		e.printf("  %s  [%s]  %s\n", f.Path, label, strings.Join(ranges, " "))
	}
	if len(generated) > 0 {
		generated = slices.Concat(
			slices.DeleteFunc(slices.Clone(generated), func(f state.File) bool { return !markerOnly(f) }),
			slices.DeleteFunc(generated, markerOnly),
		)
		e.printf("generated (%d files, not reviewed):\n", len(generated))
		for _, f := range generated {
			warn := ""
			if markerOnly(f) {
				warn = "⚠ "
			}
			e.printf("  %s%s  +%d -%d  %s\n", warn, f.Path, f.Added, f.Deleted, f.Reason)
		}
		if slices.ContainsFunc(generated, markerOnly) {
			e.println("⚠ generated only by a comment in the file: ask the human before skipping")
		}
	}
}

func printStep(e env, r *state.Review, st *state.Step) {
	pos := r.StepIndex(st.ID) + 1
	e.printf("%s %d/%d [%s] %s · %s\n", st.ID, pos, len(r.Steps), st.Status, st.Kind, st.Title)
	if st.FromRound > 0 {
		e.printf("carried: not reviewed in round %d\n", st.FromRound)
	}
	if st.Note != "" {
		e.printf("note: %s\n", st.Note)
	}
	for _, h := range st.Hunks {
		e.printf("hunk: %s %s\n", h.File, cmp.Or(h.Lines, "whole file"))
	}
	for _, h := range st.Hotspots {
		e.printf("hotspot %s: %s\n", h.Cat, h.Q)
	}
	for _, a := range st.Annotations {
		e.printf("%s %s:%d: %s\n", a.Kind, a.File, a.Line, a.Text)
	}
	if len(st.DependsOn) > 0 {
		e.printf("depends on: %s\n", strings.Join(st.DependsOn, " "))
	}
	if st.MayChange {
		e.println("may change: an earlier blocker/major touches a step this one depends on")
	}
	if st.SkipReason != "" {
		e.printf("skipped: %s\n", st.SkipReason)
	}
}

func printDiscussions(e env, r *state.Review, full bool) {
	if r.MR == nil {
		return
	}
	var open []state.Discussion
	for _, d := range r.Discussions {
		if !d.Resolved {
			open = append(open, d)
		}
	}
	if !full {
		e.printf("MR discussions: %d unresolved\n", len(open))
	}
	if mine := r.MyThreads(); len(mine) > 0 {
		answered := 0
		for _, d := range mine {
			if r.Answered(d) {
				answered++
			}
		}
		e.printf("your threads: %d open, %d answered — assess them first, see gr thread list\n",
			len(mine), answered)
	}
	for _, d := range open {
		where := ""
		if d.File != "" {
			where = fmt.Sprintf(" %s:%d", d.File, d.Line)
		}
		if full {
			e.printf(
				"@%s%s (%d replies)\n%s\n\n",
				d.Author,
				where,
				d.Replies,
				indent(d.Body, "  "),
			)
			continue
		}
		body, _, _ := strings.Cut(d.Body, "\n")
		if len(body) > 120 {
			body = body[:117] + "..."
		}
		e.printf("  @%s%s: %s\n", d.Author, where, body)
	}
}
