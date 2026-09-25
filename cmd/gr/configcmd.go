package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/aplotnikov/guided-review/internal/config"
	"github.com/aplotnikov/guided-review/internal/gitx"
	"github.com/aplotnikov/guided-review/internal/view"
)

func cmdConfig(ctx context.Context, e env, args []string) error {
	if len(args) > 0 && args[0] == "init" {
		return configInit(e)
	}
	user, path, err := config.LoadUser()
	if err != nil {
		return err
	}
	state := "found"
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		state = "not created: gr config init"
	}
	fmt.Fprintf(e.stdout, "user config: %s (%s)\n", path, state)
	merged := user
	if repo, err := gitx.Open(ctx, e.dir); err == nil {
		repoCfg, err := config.Load(repo.Dir)
		if err != nil {
			return err
		}
		repoState := "none"
		if _, err := os.Stat(filepath.Join(repo.Dir, config.FileName)); err == nil {
			repoState = "found"
		}
		fmt.Fprintf(e.stdout, "repo config: %s (%s)\n", filepath.Join(repo.Dir, config.FileName), repoState)
		merged = config.Merge(user, repoCfg)
	}
	fmt.Fprintf(e.stdout, "diff: %s\n", config.Config{Diff: merged.Diff}.DiffAlgorithm(gitx.DefaultDiffAlgorithm))
	v := merged.View
	fmt.Fprintf(e.stdout, "view: split=%v plan=%v mouse=%v context=%d style=%s\n", v.Split, !v.HidePlan, !v.NoMouse, max(v.Context, 3), cmpOr(v.Style, "monokai"))
	for lang, argv := range merged.LSP {
		fmt.Fprintf(e.stdout, "lsp %s: %s\n", lang, strings.Join(argv, " "))
	}
	if err := view.CheckKeys(merged.Keys); err != nil {
		fmt.Fprintf(e.stdout, "problem: %v\n", err)
	} else if len(merged.Keys) > 0 {
		fmt.Fprintf(e.stdout, "keys: %d actions remapped\n", len(merged.Keys))
	}
	return nil
}

func cmpOr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func configInit(e env) error {
	path, err := config.UserPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists", path)
	}
	var b strings.Builder
	b.WriteString("# guided-review user settings. Everything is commented out: uncomment what you change.\n")
	b.WriteString("# A repository's .review.yaml overrides diff and lsp.\n\n")
	b.WriteString("# view:\n#   split: false       # start in split view\n#   hide_plan: false\n")
	b.WriteString("#   no_mouse: false    # true lets the terminal select text\n#   context: 3         # lines around each change\n")
	b.WriteString("#   style: monokai     # chroma style name\n\n")
	b.WriteString("# diff: histogram      # histogram | patience | myers | minimal\n\n# lsp:\n")
	for _, lang := range []string{"go", "kotlin", "python", "typescript", "java", "rust"} {
		fmt.Fprintf(&b, "#   %s: [%s]\n", lang, quoteAll(view.DefaultServers[lang]))
	}
	b.WriteString("\n# keys:                # action: [keys]; a sequence is space separated, e.g. \"g d\"\n")
	for _, a := range view.DefaultActions() {
		fmt.Fprintf(&b, "#   %s: [%s]  # %s\n", a.Name, quoteAll(a.Keys), a.Desc)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(e.stdout, "wrote %s\n", path)
	return nil
}

func quoteAll(ss []string) string {
	q := make([]string, len(ss))
	for i, s := range ss {
		q[i] = fmt.Sprintf("%q", s)
	}
	return strings.Join(q, ", ")
}
