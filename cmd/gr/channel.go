package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aplotnikov/guided-review/internal/inbox"
	"github.com/aplotnikov/guided-review/internal/state"
)

func cmdWait(ctx context.Context, e env, args []string) error {
	fs := flag.NewFlagSet("wait", flag.ContinueOnError)
	fs.SetOutput(e.stdout)
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
		fmt.Fprintln(e.stdout, "no input yet: run gr wait again")
		return nil
	}
	for _, ev := range evs {
		fmt.Fprintln(e.stdout, ev)
	}
	return nil
}

func cmdSay(ctx context.Context, e env, args []string) error {
	text := strings.Join(args, " ")
	if text == "-" {
		data, err := io.ReadAll(e.stdin)
		if err != nil {
			return err
		}
		text = string(data)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return errors.New("usage: gr say TEXT (or - to read stdin)")
	}
	s, r, err := loadReview(ctx, e.dir)
	if err != nil {
		return err
	}
	r.Messages = append(r.Messages, state.Message{Time: time.Now(), Step: r.Current, Text: text})
	if n := len(r.Messages); n > state.MaxMessages {
		r.Messages = r.Messages[n-state.MaxMessages:]
	}
	return s.store.Save(r)
}
