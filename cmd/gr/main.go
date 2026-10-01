package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"

	"github.com/pltanton/guided-review/internal/github"
	"github.com/pltanton/guided-review/internal/gitlab"
)

const usage = `usage: gr <command> [args]

review
  init [--base REV] [--id ID] [--force] [--self] [MR-URL | PR-URL | BRANCH | BASE..HEAD]
                       BASE..HEAD = BASE...HEAD: HEAD since it forked from BASE, or
                       against BASE itself when they share no history; --base: BRANCH only
  status [--gate]
  hunks
  plan set [-f FILE]...
  step [show [ID] | next | skip --reason TEXT | goto ID]
  comment add --file F --lines N[-M] --severity S [--step ID] [--suggestion TEXT] BODY...
  comment list | resolve ID | delete ID | edit ID [--severity S] TEXT...
  note add --file F --line N | --lines N-M [--kind note|spec] [--step ID] TEXT...
  note detail --file F --line N [--step ID] TEXT... | -   details behind a note (viewer: i)
  discussions
  thread list | assess ID --propose resolve|open [--reply T] TEXT|-
                       your MR threads; the reviewer decides them in the viewer (R)
  prepare --verdict approve|changes|blocked [--decisions TEXT | --decisions-file F]
          [--approve] [--partial]
  export [--dry-run | --dir]   write the result to <git common dir>/guided-review/exports/<id>
                       and print that dir (--dir: only print it)
  mark-published       after the agent posted the export to the MR
  done
  list

viewer channel
  view [--return PANE]
  wait [--timeout 9m]
  say [--option ANSWER]... TEXT... | -   options become answer buttons in the viewer
  progress TEXT...
  idle                 Stop hook: tells the viewer the agent ended its turn

  config [init]
`

type command func(ctx context.Context, e env, args []string) error

var commands map[string]command

func init() {
	commands = map[string]command{
		"init":           cmdInit,
		"status":         cmdStatus,
		"hunks":          cmdHunks,
		"plan":           cmdPlan,
		"step":           cmdStep,
		"comment":        cmdComment,
		"note":           cmdNote,
		"discussions":    cmdDiscussions,
		"thread":         cmdThread,
		"prepare":        cmdPrepare,
		"export":         cmdExport,
		"mark-published": cmdMarkPublished,
		"done":           cmdDone,
		"list":           cmdList,
		"view":           cmdView,
		"wait":           cmdWait,
		"say":            cmdSay,
		"progress":       cmdProgress,
		"idle":           cmdIdle,
		"config":         cmdConfig,
	}
}

type env struct {
	dir      string
	cacheDir string
	stdin    io.Reader
	stdout   io.Writer
	glab     gitlab.Runner
	gh       github.Runner
}

func (e env) printf(format string, a ...any) {
	_, _ = fmt.Fprintf(e.stdout, format, a...)
}

func (e env) println(a ...any) {
	_, _ = fmt.Fprintln(e.stdout, a...)
}

func (e env) flags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(e.stdout)
	return fs
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	err := start(ctx)
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, "gr:", err)
		os.Exit(1)
	}
}

func start(ctx context.Context) error {
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return err
	}
	e := env{
		dir: dir, cacheDir: cache, stdin: os.Stdin, stdout: os.Stdout,
		glab: gitlab.Glab, gh: github.Gh,
	}
	return run(ctx, e, os.Args[1:])
}

func run(ctx context.Context, e env, args []string) error {
	if len(args) == 0 {
		e.printf("%s", usage)
		return errors.New("no command")
	}
	if cmd, ok := commands[args[0]]; ok {
		return cmd(ctx, e, args[1:])
	}
	switch args[0] {
	case "help", "-h", "--help":
		e.printf("%s", usage)
		return nil
	}
	return fmt.Errorf("unknown command %q\n\n%s", args[0], usage)
}
