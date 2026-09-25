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
	data := "domain: finance\ngenerated:\n  - api/gen/**\n"
	if err := os.WriteFile(filepath.Join(dir, config.FileName), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err = config.Load(dir)
	want := config.Config{Domain: "finance", Generated: []string{"api/gen/**"}}
	if err != nil || !reflect.DeepEqual(c, want) {
		t.Fatalf("got %+v, %v; want %+v", c, err, want)
	}
}
