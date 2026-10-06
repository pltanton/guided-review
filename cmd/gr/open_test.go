package main

import (
	"context"
	"strings"
	"testing"
)

func TestAgentPane(t *testing.T) {
	tmux := func(_ context.Context, args ...string) (string, error) {
		return "100 %1\n200 %7", nil
	}
	parents := map[int]int{900: 450, 450: 200, 200: 1}
	parent := func(_ context.Context, pid int) int { return parents[pid] }
	tests := []struct {
		name, flag, env string
		pid             int
		want            string
	}{
		{name: "flag wins", flag: "%3", env: "%4", pid: 900, want: "%3"},
		{name: "env", env: "%4", pid: 900, want: "%4"},
		{name: "process tree", pid: 900, want: "%7"},
		{name: "outside tmux", pid: 333, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := agentPane(context.Background(), tmux, tt.flag, tt.env, tt.pid, parent)
			if got != tt.want {
				t.Errorf("agentPane = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOpenRespawnsTheReviewWindow(t *testing.T) {
	h := newHarness(t)
	h.mustRun("", "init")
	var calls []string
	h.tmux = func(_ context.Context, args ...string) (string, error) {
		calls = append(calls, strings.Join(args, " "))
		switch args[0] {
		case "display-message":
			return "$2", nil
		case "list-windows":
			return "zsh\nreview-feature", nil
		}
		return "", nil
	}
	out := h.mustRun("", "open", "--return", "%5")
	if !strings.Contains(out, "review-feature") || len(calls) != 4 ||
		!strings.HasPrefix(calls[2], "respawn-window -k -t $2:review-feature") ||
		!strings.Contains(calls[2], "view --return %5") ||
		calls[3] != "select-window -t $2:review-feature" {
		t.Fatalf("open must restart the existing window: %q\n%s", calls, out)
	}
}
