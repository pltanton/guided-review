package config_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/aplotnikov/guided-review/internal/config"
)

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	c, err := config.Load(dir)
	if err != nil || !reflect.DeepEqual(c, config.Config{}) {
		t.Fatalf("missing file: %+v, %v", c, err)
	}
	data := "domain: finance\ndiff: patience\ngenerated:\n  - api/gen/**\n"
	if err := os.WriteFile(filepath.Join(dir, config.FileName), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err = config.Load(dir)
	want := config.Config{Domain: "finance", Diff: "patience", Generated: []string{"api/gen/**"}}
	if err != nil || !reflect.DeepEqual(c, want) {
		t.Fatalf("got %+v, %v; want %+v", c, err, want)
	}
}

func TestUserConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	c, path, err := config.LoadUser()
	if err != nil || path != filepath.Join(dir, "guided-review", "config.yaml") || c.Keys != nil {
		t.Fatalf("missing user config: %+v %q %v", c, path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data := "keys:\n  next-hunk: [J]\nview:\n  split: true\n  context: 6\ndiff: patience\nlsp:\n  go: [gopls, -remote=auto]\n"
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	c, _, err = config.LoadUser()
	if err != nil || !reflect.DeepEqual(c.Keys["next-hunk"], []string{"J"}) || !c.View.Split || c.View.Context != 6 || c.Diff != "patience" || c.LSP["go"][1] != "-remote=auto" {
		t.Fatalf("user config: %+v %v", c, err)
	}
	merged := config.Merge(c, config.Config{Diff: "myers", LSP: map[string][]string{"kotlin": {"kls"}}})
	if merged.Diff != "myers" || merged.LSP["go"][0] != "gopls" || merged.LSP["kotlin"][0] != "kls" {
		t.Fatalf("merge: %+v", merged)
	}
}
