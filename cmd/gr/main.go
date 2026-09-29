package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"

	"github.com/aplotnikov/guided-review/internal/gitlab"
)

const usage = `usage: gr <command> [args]

review
  init [--base REV] [--id ID] [--force] [MR-URL | BRANCH | BASE..HEAD]
  status [--gate]
  hunks
  plan set [-f FILE]
  step [show [ID] | next | skip --reason TEXT | goto ID]
  comment add --file F --lines N[-M] --severity S [--step ID] [--suggestion TEXT] BODY...
  comment list | resolve ID | delete ID | edit ID [--severity S] TEXT...
  note add --file F --line N [--kind note|spec] [--step ID] TEXT...
  discussions
  prepare --verdict approve|changes|blocked [--decisions TEXT | --decisions-file F] [--approve]
  export [--dry-run]   write the result to /tmp/guided-review/<id> for the agent to publish
  mark-published       after the agent posted the export to the MR
  done
  list

viewer channel
  view [--return PANE]
  wait [--timeout 9m]
  say TEXT... | say -
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
	dir       string
	cacheDir  string
	stdin     io.Reader
	stdout    io.Writer
	glab      gitlab.Runner
	exportDir string
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
		dir: dir, cacheDir: cache, stdin: os.Stdin, stdout: os.Stdout, glab: gitlab.Glab,
		exportDir: "/tmp/guided-review",
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
