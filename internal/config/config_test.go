package config_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/aplotnikov/guided-review/internal/config"
)

func write(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoad(t *testing.T) {
	home, repo := t.TempDir(), t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)

	got, err := config.Load(repo)
	if err != nil || !reflect.DeepEqual(got, config.Config{}) {
		t.Fatalf("no files: %+v, %v", got, err)
	}

	userPath, _ := config.UserPath()
	if want := filepath.Join(home, "guided-review", "config.yaml"); userPath != want {
		t.Fatalf("UserPath = %q, want %q", userPath, want)
	}
	write(t, userPath, "keys:\n  next-hunk: [J]\nview:\n  split: true\n  context: 6\n"+
		"diff: patience\nlsp:\n  go: [gopls]\n  kotlin: [kotlin-lsp]\n")
	write(t, filepath.Join(repo, config.RepoFile), "domain: finance\ngenerated: [api/gen/**]\n"+
		"diff: myers\nlsp:\n  kotlin: [kls]\n")

	got, err = config.Load(repo)
	want := config.Config{
		Domain:    "finance",
		Generated: []string{"api/gen/**"},
		Diff:      "myers",
		LSP:       map[string][]string{"go": {"gopls"}, "kotlin": {"kls"}},
		Keys:      map[string][]string{"next-hunk": {"J"}},
		View:      config.View{Split: true, Context: 6},
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("Load = %+v, %v\nwant %+v", got, err, want)
	}

	write(t, filepath.Join(repo, config.RepoFile), "domain: [broken\n")
	if _, err := config.Load(repo); err == nil {
		t.Fatal("broken repo file: want error")
	}
}
