package main

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/aplotnikov/guided-review/internal/diff"
	"github.com/aplotnikov/guided-review/internal/gitx"
	"github.com/aplotnikov/guided-review/internal/state"
)

type session struct {
	repo  gitx.Repo
	store state.Store
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
	key := sha1.Sum([]byte(repo.Dir))
	store := state.Store{Dir: filepath.Join(common, "guided-review"), Key: hex.EncodeToString(key[:])[:12]}
	return session{repo: repo, store: store}, nil
}

func loadReview(ctx context.Context, dir string) (session, *state.Review, error) {
	s, err := openSession(ctx, dir)
	if err != nil {
		return session{}, nil, err
	}
	r, err := s.store.LoadCurrent()
	return s, r, err
}

func (s session) diffFiles(ctx context.Context, r *state.Review) ([]diff.File, error) {
	raw, err := s.repo.Diff(ctx, r.DiffBase(), r.HeadSHA)
	if err != nil {
		return nil, err
	}
	return diff.Parse(raw)
}

func short(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}

func printHunks(w io.Writer, r *state.Review, files []diff.File) {
	var generated []state.File
	fmt.Fprintln(w, "hunks (new-file line ranges):")
	for _, f := range files {
		rf := r.File(f.Path)
		if rf == nil {
			continue
		}
		if rf.Tier == state.TierGenerated {
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
		fmt.Fprintf(w, "  %s  [%s]  %s\n", f.Path, label, strings.Join(ranges, " "))
	}
	if len(generated) > 0 {
		fmt.Fprintf(w, "generated (%d files, not reviewed):\n", len(generated))
		for _, f := range generated {
			fmt.Fprintf(w, "  %s  +%d -%d\n", f.Path, f.Added, f.Deleted)
		}
	}
}

func printStep(w io.Writer, r *state.Review, st *state.Step) {
	fmt.Fprintf(w, "%s %d/%d [%s] %s · %s\n", st.ID, r.StepIndex(st.ID)+1, len(r.Steps), st.Status, st.Kind, st.Title)
	if st.Note != "" {
		fmt.Fprintf(w, "note: %s\n", st.Note)
	}
	for _, h := range st.Hunks {
		lines := h.Lines
		if lines == "" {
			lines = "whole file"
		}
		fmt.Fprintf(w, "hunk: %s %s\n", h.File, lines)
	}
	for _, h := range st.Hotspots {
		fmt.Fprintf(w, "hotspot %s: %s\n", h.Cat, h.Q)
	}
	for _, a := range st.Annotations {
		fmt.Fprintf(w, "%s %s:%d: %s\n", a.Kind, a.File, a.Line, a.Text)
	}
	if len(st.DependsOn) > 0 {
		fmt.Fprintf(w, "depends on: %s\n", strings.Join(st.DependsOn, " "))
	}
	if st.MayChange {
		fmt.Fprintln(w, "may change: an earlier blocker/major touches a step this one depends on")
	}
	if st.SkipReason != "" {
		fmt.Fprintf(w, "skipped: %s\n", st.SkipReason)
	}
}
