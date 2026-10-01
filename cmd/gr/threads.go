package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/pltanton/guided-review/internal/state"
)

const threadUsage = "usage: gr thread list | " +
	"gr thread assess ID --propose resolve|open [--reply TEXT] TEXT...|- | " +
	"gr thread decide ID --verdict resolve|open|none [--reply TEXT]"

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
	if len(args) < 2 || args[0] != "assess" && args[0] != "decide" {
		return errors.New(threadUsage)
	}
	var d *state.Discussion
	var t state.Thread
	_, _, err := updateReview(ctx, e.dir, func(_ session, r *state.Review) error {
		d = r.Discussion(args[1])
		if d == nil {
			return fmt.Errorf("no thread %q: run gr discussions to refresh", args[1])
		}
		fs := e.flags("thread " + args[0])
		propose := fs.String("propose", "", "resolve|open: what the agent suggests")
		verdict := fs.String("verdict", "", "resolve|open|none: the reviewer's decision")
		reply := fs.String("reply", "", "text to post in the thread")
		if err := fs.Parse(args[2:]); err != nil {
			return err
		}
		t = r.ThreadState(*d)
		if args[0] == "assess" {
			text := strings.Join(fs.Args(), " ")
			if text == "-" {
				data, err := readInput(e, "-")
				if err != nil {
					return err
				}
				text = string(data)
			}
			if *propose != state.VerdictResolve && *propose != state.VerdictOpen {
				return errors.New("--propose must be resolve or open")
			}
			t.Assessment, t.Proposed = strings.TrimSpace(text), *propose
			t.ProposedReply = strings.TrimSpace(*reply)
		} else {
			switch *verdict {
			case state.VerdictResolve, state.VerdictOpen:
				t.Verdict, t.Reply = *verdict, strings.TrimSpace(*reply)
			case "none":
				t.Verdict, t.Reply = "", ""
			default:
				return errors.New("--verdict must be resolve, open or none")
			}
			if c := r.CommentFor(*d); c != nil {
				c.Resolved = t.Verdict == state.VerdictResolve
			}
		}
		r.SetThread(t)
		return nil
	})
	if err != nil {
		return err
	}
	e.printf("thread %s: %s\n", d.ID, threadStatus(t))
	return nil
}

func threadStatus(t state.Thread) string {
	switch {
	case t.Verdict != "":
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
		if r.Answered(d) && r.ThreadState(d).Verdict == "" {
			ids = append(ids, d.ID)
		}
	}
	return ids
}

func openThreads(r *state.Review) []string {
	var ids []string
	for _, d := range r.MyThreads() {
		if r.ThreadState(d).Verdict != state.VerdictResolve {
			ids = append(ids, d.ID)
		}
	}
	return ids
}
