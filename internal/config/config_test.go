package config_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/pltanton/guided-review/internal/config"
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

func TestSaveTheme(t *testing.T) {
	tests := []struct {
		name string
		give string
		want string
	}{
		{"no file", "", "view:\n  theme: nord\n"},
		{"only comments", "# view:\n#   split: false\n", "# view:\n#   split: false\n\nview:\n  theme: nord\n"},
		{"keeps other settings", "diff: patience\nview:\n  split: true\n",
			"diff: patience\nview:\n    split: true\n    theme: nord\n"},
		{"replaces the theme", "view:\n  theme: dracula\n", "view:\n    theme: nord\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if tt.give != "" {
				if err := os.WriteFile(path, []byte(tt.give), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if err := config.SaveTheme(path, "nord"); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Errorf("SaveTheme wrote\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("- a list\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := config.SaveTheme(path, "nord"); err == nil {
		t.Error("SaveTheme on a list: want an error")
	}
}
