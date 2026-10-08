package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/pltanton/guided-review/internal/state"
)

const threadUsage = "usage: gr thread list | " +
	"gr thread assess ID --propose resolve|open [--reply TEXT] TEXT...|- " +
	"(the reviewer decides with R in the viewer)"

func cmdThread(ctx context.Context, e env, args []string) error {
	if len(args) == 0 {
		args = []string{"list"}
	}
	if args[0] == "list" {
		_, r, err := loadReview(ctx, e.dir)
		if err != nil {
			return err
		}
		printThreads(e, r)
		return nil
	}
	if len(args) < 2 || args[0] != "assess" {
		return errors.New(threadUsage)
	}
	id := args[1]
	fs := e.flags("thread assess")
	propose := fs.String("propose", "", "resolve|open: what the agent suggests")
	reply := fs.String("reply", "", "text to post in the thread")
	if err := fs.Parse(args[2:]); err != nil {
		return err
	}
	if *propose != state.VerdictResolve && *propose != state.VerdictOpen {
		return errors.New("--propose must be resolve or open")
	}
	text := strings.Join(fs.Args(), " ")
	if text == "-" {
		data, err := readInput(e, "-")
		if err != nil {
			return err
		}
		text = string(data)
	}
	var t state.Thread
	_, _, err := updateReview(ctx, e.dir, func(_ session, r *state.Review) error {
		d := r.Discussion(id)
		if d == nil {
			return fmt.Errorf("no thread %q: run gr discussions to refresh", id)
		}
		t = r.ThreadState(*d)
		t.Assessment, t.Proposed = strings.TrimSpace(text), *propose
		t.ProposedReply = strings.TrimSpace(*reply)
		r.SetThread(t)
		return nil
	})
	if err != nil {
		return err
	}
	e.printf("thread %s: %s\n", id, threadStatus(t))
	return nil
}

func threadStatus(t state.Thread) string {
	switch {
	case t.Decided():
		return "decided " + t.Verdict
	case t.Proposed != "":
		return "agent proposes " + t.Proposed
	}
	return "undecided"
}

func printThreads(e env, r *state.Review) {
	threads := r.MyThreads()
	if len(threads) == 0 {
		e.println("no open threads of yours")
		return
	}
	for _, d := range threads {
		t := r.ThreadState(d)
		where := "general"
		if d.File != "" {
			where = fmt.Sprintf("%s:%d", d.File, d.Line)
		}
		status := "no reply"
		if r.Answered(d) {
			status = "answered"
		}
		e.printf("thread %s  %s  %s, %s\n", d.ID, where, status, threadStatus(t))
		for _, n := range d.Notes {
			who := "@" + n.Author
			if n.Author == r.MR.Me {
				who = "you"
			}
			e.printf("  %s:\n%s\n", who, indent(strings.TrimSpace(n.Body), "    "))
		}
		if t.Assessment != "" {
			e.printf("  agent (%s): %s\n", t.Proposed, t.Assessment)
		}
		if t.Reply != "" {
			e.printf("  reply: %s\n", t.Reply)
		}
		e.println()
	}
}

func undecidedThreads(r *state.Review) []string {
	var ids []string
	for _, d := range r.MyThreads() {
		if r.Answered(d) && !r.ThreadState(d).Decided() {
			ids = append(ids, d.ID)
		}
	}
	return ids
}
