package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"strconv"
	"strings"

	"github.com/aplotnikov/guided-review/internal/plan"
	"github.com/aplotnikov/guided-review/internal/state"
)

func cmdComment(ctx context.Context, e env, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: gr comment add|list|resolve")
	}
	s, r, err := loadReview(ctx, e.dir)
	if err != nil {
		return err
	}
	switch args[0] {
	case "list":
		for _, c := range r.Comments {
			status := ""
			if c.Resolved {
				status = "  [resolved]"
			}
			fmt.Fprintf(e.stdout, "#%d %s %s %s:%s  %s%s\n", c.ID, c.Severity, c.Step, c.File, c.Lines, c.Body, status)
			if c.Suggestion != "" {
				fmt.Fprintf(e.stdout, "   suggestion:\n%s\n", indent(c.Suggestion, "     "))
			}
		}
		return nil
	case "resolve":
		if len(args) < 2 {
			return errors.New("usage: gr comment resolve ID")
		}
		id, err := strconv.Atoi(strings.TrimPrefix(args[1], "#"))
		if err != nil {
			return fmt.Errorf("comment id %q: %w", args[1], err)
		}
		if err := plan.ResolveComment(r, id); err != nil {
			return err
		}
		if err := s.store.Save(r); err != nil {
			return err
		}
		fmt.Fprintf(e.stdout, "comment #%d resolved\n", id)
		return nil
	case "add":
		fs := flag.NewFlagSet("comment add", flag.ContinueOnError)
		fs.SetOutput(e.stdout)
		file := fs.String("file", "", "file path as in the diff")
		lines := fs.String("lines", "", "new-file lines, N or N-M")
		severity := fs.String("severity", "", "blocker|major|minor|nit")
		step := fs.String("step", "", "step id (default: current)")
		suggestion := fs.String("suggestion", "", "replacement text for the lines")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		c, imp, err := plan.AddComment(r, state.Comment{
			Step: *step, File: *file, Lines: *lines, Severity: state.Severity(*severity),
			Body: strings.Join(fs.Args(), " "), Suggestion: *suggestion,
		})
		if err != nil {
			return err
		}
		if err := s.store.Save(r); err != nil {
			return err
		}
		fmt.Fprintf(e.stdout, "comment #%d %s %s:%s\n", c.ID, c.Severity, c.File, c.Lines)
		if len(imp.Stale) > 0 {
			fmt.Fprintf(e.stdout, "stale (depend on a blocked step): %s\n", strings.Join(imp.Stale, " "))
		}
		if len(imp.MayChange) > 0 {
			fmt.Fprintf(e.stdout, "may change: %s\n", strings.Join(imp.MayChange, " "))
		}
		return nil
	}
	return fmt.Errorf("unknown comment command %q", args[0])
}

func indent(s, prefix string) string {
	return prefix + strings.ReplaceAll(s, "\n", "\n"+prefix)
}
