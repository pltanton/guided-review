package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"

	"github.com/aplotnikov/guided-review/internal/gitlab"
)

const usage = `usage: gr <command> [args]

  init [--base REV] [--id ID] [--force] [MR-URL | BRANCH | BASE..HEAD]
  list
  sync
  done
  hunks
  plan set [-f FILE]
  step [show [ID] | next | skip --reason TEXT | goto ID]
  comment add --file F --lines N[-M] --severity blocker|major|minor|nit [--step ID] [--suggestion TEXT] BODY...
  comment list | resolve ID
  note add --file F --line N [--kind note|spec] [--step ID] TEXT...
  status [--gate]
  view
`

type env struct {
	dir      string
	cacheDir string
	stdin    io.Reader
	stdout   io.Writer
	glab     gitlab.Runner
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	dir, err := os.Getwd()
	if err == nil {
		var cache string
		if cache, err = os.UserCacheDir(); err == nil {
			err = run(ctx, env{dir: dir, cacheDir: cache, stdin: os.Stdin, stdout: os.Stdout, glab: gitlab.Glab}, os.Args[1:])
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "gr:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, e env, args []string) error {
	if len(args) == 0 {
		fmt.Fprint(e.stdout, usage)
		return errors.New("no command")
	}
	switch args[0] {
	case "init":
		return cmdInit(ctx, e, args[1:])
	case "list":
		return cmdList(ctx, e)
	case "sync":
		return cmdSync(ctx, e)
	case "done":
		return cmdDone(ctx, e)
	case "hunks":
		return cmdHunks(ctx, e)
	case "plan":
		return cmdPlan(ctx, e, args[1:])
	case "step":
		return cmdStep(ctx, e, args[1:])
	case "note":
		return cmdNote(ctx, e, args[1:])
	case "comment":
		return cmdComment(ctx, e, args[1:])
	case "status":
		return cmdStatus(ctx, e, args[1:])
	case "view":
		return cmdView(ctx, e)
	case "help", "-h", "--help":
		fmt.Fprint(e.stdout, usage)
		return nil
	}
	return fmt.Errorf("unknown command %q\n\n%s", args[0], usage)
}
