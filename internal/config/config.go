package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const FileName = ".review.yaml"

type Config struct {
	Domain    string              `yaml:"domain"`
	Diff      string              `yaml:"diff"`
	Generated []string            `yaml:"generated"`
	LSP       map[string][]string `yaml:"lsp"`
}

func (c Config) DiffAlgorithm(fallback string) string {
	if c.Diff == "" {
		return fallback
	}
	return c.Diff
}

func Load(root string) (Config, error) {
	data, err := os.ReadFile(filepath.Join(root, FileName))
	if errors.Is(err, fs.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, err
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return Config{}, fmt.Errorf("%s: %w", FileName, err)
	}
	return c, nil
}

type ViewConfig struct {
	Split    bool   `yaml:"split"`
	HidePlan bool   `yaml:"hide_plan"`
	NoMouse  bool   `yaml:"no_mouse"`
	Context  int    `yaml:"context"`
	Style    string `yaml:"style"`
}

type UserConfig struct {
	Keys map[string][]string `yaml:"keys"`
	View ViewConfig          `yaml:"view"`
	Diff string              `yaml:"diff"`
	LSP  map[string][]string `yaml:"lsp"`
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

func LoadUser() (UserConfig, string, error) {
	path, err := UserPath()
	if err != nil {
		return UserConfig{}, "", err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return UserConfig{}, path, nil
	}
	if err != nil {
		return UserConfig{}, path, err
	}
	var c UserConfig
	if err := yaml.Unmarshal(data, &c); err != nil {
		return UserConfig{}, path, fmt.Errorf("%s: %w", path, err)
	}
	return c, path, nil
}

func Merge(user UserConfig, repo Config) UserConfig {
	if repo.Diff != "" {
		user.Diff = repo.Diff
	}
	if len(repo.LSP) > 0 {
		lsp := map[string][]string{}
		for k, v := range user.LSP {
			lsp[k] = v
		}
		for k, v := range repo.LSP {
			lsp[k] = v
		}
		user.LSP = lsp
	}
	return user
}
