package main

import (
	"context"
	"flag"

	"github.com/aplotnikov/guided-review/internal/view"
)

func cmdView(ctx context.Context, e env, args []string) error {
	fs := flag.NewFlagSet("view", flag.ContinueOnError)
	fs.SetOutput(e.stdout)
	returnPane := fs.String("return", "", "tmux pane to focus when the viewer exits (the agent's $TMUX_PANE)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	s, err := openSession(ctx, e.dir)
	if err != nil {
		return err
	}
	return view.Run(ctx, s.store, s.repo, *returnPane)
}
