package main

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/pltanton/guided-review/internal/inbox"
	"github.com/pltanton/guided-review/internal/state"
	"github.com/pltanton/guided-review/internal/view"
)

func cmdView(ctx context.Context, e env, args []string) error {
	fs := e.flags("view")
	pane := fs.String(
		"return",
		"",
		"tmux pane to focus when the viewer exits (the agent's $TMUX_PANE)",
	)
	if err := fs.Parse(args); err != nil {
		return err
	}
	s, err := openSession(ctx, e.dir)
	if err != nil {
		return err
	}
	return view.Run(
		ctx,
		view.Options{Store: s.store, Repo: s.repo, Config: s.cfg, ReturnPane: *pane},
	)
}

func cmdWait(ctx context.Context, e env, args []string) error {
	fs := e.flags("wait")
	timeout := fs.Duration("timeout", 9*time.Minute, "how long to wait for viewer input")
	if err := fs.Parse(args); err != nil {
		return err
	}
	s, r, err := loadReview(ctx, e.dir)
	if err != nil {
		return err
	}
	evs, err := inbox.Wait(ctx, s.store.ReviewDir(r.ID), *timeout, 200*time.Millisecond)
	if err != nil {
		return err
	}
	if len(evs) == 0 {
		e.println("no input yet: run gr wait again")
	}
	for _, ev := range evs {
		e.println(ev)
	}
	return nil
}

func cmdSay(ctx context.Context, e env, args []string) error {
	var options []string
	for len(args) > 1 && args[0] == "--option" {
		options, args = append(options, args[1]), args[2:]
	}
	text := strings.Join(args, " ")
	if text == "-" {
		data, err := readInput(e, "-")
		if err != nil {
			return err
		}
		text = string(data)
	}
	if text = strings.TrimSpace(text); text == "" {
		return errors.New("usage: gr say [--option ANSWER]... TEXT (or - to read stdin)")
	}
	return update(ctx, e, func(r *state.Review) {
		msg := state.Message{Time: time.Now(), Step: r.Current, Text: text, Options: options}
		r.Messages = append(r.Messages, msg)
		r.Messages = r.Messages[max(len(r.Messages)-state.MaxMessages, 0):]
		r.Progress = nil
	})
}

func cmdProgress(ctx context.Context, e env, args []string) error {
	text := strings.TrimSpace(strings.Join(args, " "))
	if text == "" {
		return errors.New("usage: gr progress TEXT")
	}
	return update(ctx, e, func(r *state.Review) {
		r.Progress = &state.Progress{Text: text, Time: time.Now()}
	})
}

func update(ctx context.Context, e env, apply func(*state.Review)) error {
	_, _, err := updateReview(ctx, e.dir, func(_ session, r *state.Review) error {
		apply(r)
		return nil
	})
	return err
}

// cmdIdle runs as a Claude Code Stop hook in every session, so outside a review it
// must succeed silently.
func cmdIdle(ctx context.Context, e env, _ []string) error {
	s, err := openSession(ctx, e.dir)
	if err != nil {
		return nil
	}
	id, err := s.store.Current()
	if err != nil {
		return nil
	}
	return inbox.MarkIdle(s.store.ReviewDir(id))
}
