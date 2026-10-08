package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/pltanton/guided-review/internal/publish"
)

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
	for _, tool := range []string{publish.Tool(r.MR.Provider), "jq"} {
		if _, err := exec.LookPath(tool); err != nil {
			return fmt.Errorf("publishing needs %s on PATH", tool)
		}
	}
	script, err := publish.Script(r.MR.Provider)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp("", "gr-publish-*.sh")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, err := f.Write(script); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "bash", f.Name())
	cmd.Dir, cmd.Stdout, cmd.Stderr = e.dir, e.stdout, e.stdout
	cmd.Env = os.Environ()
	if self, err := os.Executable(); err == nil {
		cmd.Env = append(cmd.Env, "PATH="+filepath.Dir(self)+string(os.PathListSeparator)+
			os.Getenv("PATH"))
	}
	return cmd.Run()
}
