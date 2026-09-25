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
	Domain    string   `yaml:"domain"`
	Diff      string   `yaml:"diff"`
	Generated []string `yaml:"generated"`
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
