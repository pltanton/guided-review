package main

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aplotnikov/guided-review/internal/config"
	"github.com/aplotnikov/guided-review/internal/gitx"
	"github.com/aplotnikov/guided-review/internal/view"
)

func cmdConfig(ctx context.Context, e env, args []string) error {
	path, err := config.UserPath()
	if err != nil {
		return err
	}
	if len(args) > 0 && args[0] == "init" {
		return configInit(e, path)
	}
	s, err := openSession(ctx, e.dir)
	if err != nil {
		return err
	}
	exists := func(p string) string {
		if _, err := os.Stat(p); err != nil {
			return "not created"
		}
		return "found"
	}
	repoFile := filepath.Join(s.repo.Dir, config.RepoFile)
	e.printf("user config: %s (%s)\n", path, exists(path))
	e.printf("repo config: %s (%s)\n", repoFile, exists(repoFile))
	e.printf("diff: %s\n", cmp.Or(s.cfg.Diff, gitx.DefaultDiffAlgorithm))
	v := s.cfg.View
	e.printf("view: split=%v plan=%v mouse=%v context=%d style=%s\n",
		v.Split, !v.HidePlan, !v.NoMouse, cmp.Or(v.Context, 3), cmp.Or(v.Style, "monokai"))
	for lang, argv := range s.cfg.LSP {
		e.printf("lsp %s: %s\n", lang, strings.Join(argv, " "))
	}
	if err := view.CheckKeys(s.cfg.Keys); err != nil {
		e.printf("problem: %v\n", err)
	}
	return nil
}

func configInit(e env, path string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists", path)
	}
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }
	w("# guided-review user settings. Everything is commented out: uncomment what you change.\n")
	w("# A repository's .review.yaml overrides diff and lsp.\n\n")
	w("# view:\n#   split: false       # start in split view\n#   hide_plan: false\n")
	w("#   no_mouse: false    # true lets the terminal select text\n")
	w(
		"#   context: 3         # lines around each change\n#   style: monokai     # chroma style\n\n",
	)
	w("# diff: histogram      # histogram | patience | myers | minimal\n\n# lsp:\n")
	for _, lang := range []string{"go", "kotlin", "python", "typescript", "java", "rust"} {
		w("#   %s: [%s]\n", lang, quoteAll(view.DefaultServers[lang]))
	}
	w("\n# keys:                # action: [keys]; a sequence is space separated, e.g. \"g d\"\n")
	for _, a := range view.DefaultActions() {
		w("#   %s: [%s]  # %s\n", a.Name, quoteAll(a.Keys), a.Desc)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return err
	}
	e.printf("wrote %s\n", path)
	return nil
}

func quoteAll(ss []string) string {
	q := make([]string, len(ss))
	for i, s := range ss {
		q[i] = fmt.Sprintf("%q", s)
	}
	return strings.Join(q, ", ")
}
