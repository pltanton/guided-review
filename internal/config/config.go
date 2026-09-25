package config

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const RepoFile = ".review.yaml"

type View struct {
	Split    bool   `yaml:"split"`
	HidePlan bool   `yaml:"hide_plan"`
	NoMouse  bool   `yaml:"no_mouse"`
	Context  int    `yaml:"context"`
	Style    string `yaml:"style"`
}

type Config struct {
	Domain    string              `yaml:"domain"`
	Generated []string            `yaml:"generated"`
	Diff      string              `yaml:"diff"`
	LSP       map[string][]string `yaml:"lsp"`
	Keys      map[string][]string `yaml:"keys"`
	View      View                `yaml:"view"`
}

func UserPath() (string, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "guided-review", "config.yaml"), nil
}

func Load(repoRoot string) (Config, error) {
	var c Config
	path, err := UserPath()
	if err != nil {
		return c, err
	}
	if err := read(path, &c); err != nil {
		return c, err
	}
	var repo Config
	if err := read(filepath.Join(repoRoot, RepoFile), &repo); err != nil {
		return c, err
	}
	c.Domain, c.Generated = repo.Domain, repo.Generated
	if repo.Diff != "" {
		c.Diff = repo.Diff
	}
	if len(repo.LSP) > 0 {
		lsp := maps.Clone(c.LSP)
		if lsp == nil {
			lsp = map[string][]string{}
		}
		maps.Copy(lsp, repo.LSP)
		c.LSP = lsp
	}
	return c, nil
}

func read(path string, c *Config) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(data, c); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}
