package config

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const RepoFile = ".review.yaml"

type View struct {
	Split   bool   `yaml:"split"`
	NoMouse bool   `yaml:"no_mouse"`
	NoWrap  bool   `yaml:"no_wrap"`
	Context int    `yaml:"context"`
	Style   string `yaml:"style"`
	Theme   string `yaml:"theme"`
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

func SaveTheme(path, theme string) error {
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if len(doc.Content) == 0 {
		text := strings.TrimRight(string(data), "\n")
		if text != "" {
			text += "\n\n"
		}
		return writeConfig(path, []byte(text+"view:\n  theme: "+theme+"\n"))
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return fmt.Errorf("%s: the top level is not a mapping", path)
	}
	setKey(setKey(root, "view", yaml.MappingNode), "theme", yaml.ScalarNode).Value = theme
	out, err := yaml.Marshal(&doc)
	if err != nil {
		return err
	}
	return writeConfig(path, out)
}

func writeConfig(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func setKey(m *yaml.Node, key string, kind yaml.Kind) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			if v := m.Content[i+1]; v.Kind == kind {
				return v
			}
			m.Content[i+1] = &yaml.Node{Kind: kind}
			return m.Content[i+1]
		}
	}
	v := &yaml.Node{Kind: kind}
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, v)
	return v
}
