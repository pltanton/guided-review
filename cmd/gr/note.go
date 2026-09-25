package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"strings"

	"github.com/aplotnikov/guided-review/internal/plan"
	"github.com/aplotnikov/guided-review/internal/state"
)

func cmdNote(ctx context.Context, e env, args []string) error {
	if len(args) == 0 || args[0] != "add" {
		return errors.New("usage: gr note add --file F --line N [--kind note|spec] [--step ID] TEXT")
	}
	fs := flag.NewFlagSet("note add", flag.ContinueOnError)
	fs.SetOutput(e.stdout)
	file := fs.String("file", "", "file path as in the diff")
	line := fs.Int("line", 0, "new-file line")
	kind := fs.String("kind", "note", "note|spec")
	step := fs.String("step", "", "step id (default: current)")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	s, r, err := loadReview(ctx, e.dir)
	if err != nil {
		return err
	}
	a := state.Annotation{File: *file, Line: *line, Kind: *kind, Text: strings.Join(fs.Args(), " ")}
	if err := plan.AddNote(r, *step, a); err != nil {
		return err
	}
	if err := s.store.Save(r); err != nil {
		return err
	}
	id := *step
	if id == "" {
		id = r.Current
	}
	fmt.Fprintf(e.stdout, "note %s %s:%d\n", id, a.File, a.Line)
	return nil
}
