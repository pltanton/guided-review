package main

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/pltanton/guided-review/internal/state"
)

//go:embed publish.sh
var publishScript string

func cmdPublish(ctx context.Context, e env, _ []string) error {
	_, r, err := loadReview(ctx, e.dir)
	if err != nil {
		return err
	}
	if r.MR == nil {
		return errors.New("a local review has nothing to publish: fixes.json is the result")
	}
	if r.Publish == nil || r.Publish.Export == "" {
		return errors.New("nothing exported yet: run gr export first")
	}
	tool := "glab"
	if r.MR.Provider == state.ProviderGitHub {
		tool = "gh"
	}
	for _, name := range []string{tool, "jq"} {
		if _, err := exec.LookPath(name); err != nil {
			return fmt.Errorf("publishing needs %s on PATH", name)
		}
	}
	cmd := exec.CommandContext(ctx, "bash", "-c", publishScript)
	cmd.Dir, cmd.Stdout, cmd.Stderr = e.dir, e.stdout, e.stdout
	if self, err := os.Executable(); err == nil {
		path := filepath.Dir(self) + string(os.PathListSeparator) + os.Getenv("PATH")
		cmd.Env = append(os.Environ(), "PATH="+path)
	}
	return cmd.Run()
}
