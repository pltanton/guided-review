package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
)

type tmuxRunner func(ctx context.Context, args ...string) (string, error)

func runTmux(ctx context.Context, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, "tmux", args...).Output()
	return strings.TrimSpace(string(out)), err
}

func parentPID(ctx context.Context, pid int) int {
	out, err := exec.CommandContext(ctx, "ps", "-o", "ppid=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return 0
	}
	ppid, _ := strconv.Atoi(strings.TrimSpace(string(out)))
	return ppid
}

func cmdPane(ctx context.Context, e env, _ []string) error {
	tmux := e.tmux
	if tmux == nil {
		tmux = runTmux
	}
	pane := agentPane(ctx, tmux, "", os.Getenv("TMUX_PANE"), os.Getppid(), parentPID)
	if pane == "" {
		e.println("not in tmux")
		return nil
	}
	e.println(pane)
	return nil
}

func cmdOpen(ctx context.Context, e env, args []string) error {
	fs := e.flags("open")
	ret := fs.String("return", "", "the agent's tmux pane (default: found from $TMUX_PANE or the process tree)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	_, r, err := loadReview(ctx, e.dir)
	if err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	tmux := e.tmux
	if tmux == nil {
		tmux = runTmux
	}
	pane := agentPane(ctx, tmux, *ret, os.Getenv("TMUX_PANE"), os.Getppid(), parentPID)
	if pane == "" {
		e.printf("not in tmux: ask the human to run the viewer in another terminal:\n"+
			"  cd %s && %s view\n", e.dir, exe)
		return nil
	}
	session, err := tmux(ctx, "display-message", "-p", "-t", pane, "#{session_id}")
	if err != nil {
		return fmt.Errorf("tmux pane %s: %w", pane, err)
	}
	name := "review-" + r.ID
	command := fmt.Sprintf("%s view --return %s", shellQuote(exe), pane)
	windows, _ := tmux(ctx, "list-windows", "-t", session, "-F", "#{window_name}")
	target := session + ":" + name
	if slices.Contains(strings.Split(windows, "\n"), name) {
		if _, err := tmux(ctx, "respawn-window", "-k", "-t", target, "-c", e.dir, command); err != nil {
			return err
		}
		_, err = tmux(ctx, "select-window", "-t", target)
	} else {
		_, err = tmux(ctx, "new-window", "-t", session+":", "-n", name, "-c", e.dir, command)
	}
	if err != nil {
		return err
	}
	e.printf("viewer opened in tmux window %s (returns to %s)\n", name, pane)
	return nil
}

// $TMUX can be missing from the agent's shell even when it runs inside tmux, so the pane is
// also found by walking up the process tree to a pane's shell.
func agentPane(
	ctx context.Context,
	tmux tmuxRunner,
	flag, env string,
	pid int,
	parent func(context.Context, int) int,
) string {
	if flag != "" {
		return flag
	}
	if env != "" {
		return env
	}
	out, err := tmux(ctx, "list-panes", "-a", "-F", "#{pane_pid} #{pane_id}")
	if err != nil {
		return ""
	}
	panes := map[int]string{}
	for _, l := range strings.Split(out, "\n") {
		p, id, ok := strings.Cut(l, " ")
		if n, err := strconv.Atoi(p); ok && err == nil {
			panes[n] = id
		}
	}
	for seen := 0; pid > 1 && seen < 64; seen++ {
		if id, ok := panes[pid]; ok {
			return id
		}
		pid = parent(ctx, pid)
	}
	return ""
}

func shellQuote(s string) string {
	if s != "" && !strings.ContainsAny(s, " '\"\\$`!*?[]{}()<>|&;#~") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
