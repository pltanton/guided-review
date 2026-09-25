package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/aplotnikov/guided-review/internal/plan"
	"github.com/aplotnikov/guided-review/internal/state"
)

func cmdStep(ctx context.Context, e env, args []string) error {
	if len(args) == 0 {
		args = []string{"show"}
	}
	s, r, err := loadReview(ctx, e.dir)
	if err != nil {
		return err
	}
	if len(r.Steps) == 0 {
		return errors.New("no plan yet: pipe a plan to gr plan set")
	}
	switch args[0] {
	case "show":
		id := r.Current
		if len(args) > 1 {
			id = args[1]
		}
		st := r.Step(id)
		if st == nil {
			return fmt.Errorf("no step %q", id)
		}
		printStep(e.stdout, r, st)
		return nil
	case "next":
		st, err := plan.Next(r)
		return saveAndShow(e.stdout, s, r, st, err)
	case "skip":
		fs := flag.NewFlagSet("step skip", flag.ContinueOnError)
		fs.SetOutput(e.stdout)
		reason := fs.String("reason", "", "why the step is skipped (required)")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		st, err := plan.Skip(r, *reason)
		return saveAndShow(e.stdout, s, r, st, err)
	case "goto":
		if len(args) < 2 {
			return errors.New("usage: gr step goto ID")
		}
		if err := plan.Goto(r, args[1]); err != nil {
			return err
		}
		return saveAndShow(e.stdout, s, r, r.Step(r.Current), nil)
	}
	return fmt.Errorf("unknown step command %q", args[0])
}

func saveAndShow(w io.Writer, s session, r *state.Review, st *state.Step, err error) error {
	if err != nil && !errors.Is(err, plan.ErrDone) {
		return err
	}
	if err := s.store.Save(r); err != nil {
		return err
	}
	if st == nil {
		fmt.Fprintln(w, "all steps reviewed: run gr status")
		return nil
	}
	printStep(w, r, st)
	return nil
}
