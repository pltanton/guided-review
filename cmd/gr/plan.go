package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/aplotnikov/guided-review/internal/plan"
)

func cmdPlan(ctx context.Context, e env, args []string) error {
	if len(args) == 0 || args[0] != "set" {
		return errors.New("usage: gr plan set [-f FILE]")
	}
	fs := flag.NewFlagSet("plan set", flag.ContinueOnError)
	fs.SetOutput(e.stdout)
	file := fs.String("f", "", "plan file (default: stdin)")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	var (
		data []byte
		err  error
	)
	if *file != "" {
		path := *file
		if !filepath.IsAbs(path) {
			path = filepath.Join(e.dir, path)
		}
		data, err = os.ReadFile(path)
	} else {
		data, err = io.ReadAll(e.stdin)
	}
	if err != nil {
		return err
	}
	p, err := plan.Parse(data)
	if err != nil {
		return err
	}
	s, r, err := loadReview(ctx, e.dir)
	if err != nil {
		return err
	}
	files, err := s.diffFiles(ctx, r)
	if err != nil {
		return err
	}
	if errs := plan.Validate(p, r, files); len(errs) > 0 {
		msgs := make([]string, len(errs))
		for i, err := range errs {
			msgs[i] = err.Error()
		}
		return fmt.Errorf("plan rejected:\n  - %s", strings.Join(msgs, "\n  - "))
	}
	plan.Apply(r, p)
	if err := s.store.Save(r); err != nil {
		return err
	}
	fmt.Fprintf(e.stdout, "plan accepted: %d steps\n\n", len(r.Steps))
	printStep(e.stdout, r, r.Step(r.Current))
	return nil
}
