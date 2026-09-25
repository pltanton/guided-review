package state

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	FileName      = "state.yaml"
	currentPrefix = "current"
)

var ErrNoReview = errors.New("no active review: run gr init")

type Store struct {
	Dir string
	Key string
}

func IsCurrentFile(name string) bool {
	return strings.HasPrefix(name, currentPrefix)
}

func (s Store) currentPath() string {
	name := currentPrefix
	if s.Key != "" {
		name += "-" + s.Key
	}
	return filepath.Join(s.Dir, name)
}

func (s Store) path(id string) string {
	return filepath.Join(s.Dir, id, FileName)
}

func (s Store) Exists(id string) bool {
	_, err := os.Stat(s.path(id))
	return err == nil
}

func (s Store) Load(id string) (*Review, error) {
	data, err := os.ReadFile(s.path(id))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("review %s: %w", id, ErrNoReview)
	}
	if err != nil {
		return nil, err
	}
	var r Review
	if err := yaml.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("%s: %w", s.path(id), err)
	}
	return &r, nil
}

func (s Store) Save(r *Review) error {
	data, err := yaml.Marshal(r)
	if err != nil {
		return err
	}
	return writeAtomic(s.path(r.ID), data)
}

func (s Store) SetCurrent(id string) error {
	return writeAtomic(s.currentPath(), []byte(id+"\n"))
}

func (s Store) ClearCurrent() error {
	err := os.Remove(s.currentPath())
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

func (s Store) List() ([]string, error) {
	entries, err := os.ReadDir(s.Dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, e := range entries {
		if e.IsDir() && s.Exists(e.Name()) {
			ids = append(ids, e.Name())
		}
	}
	return ids, nil
}

func (s Store) Current() (string, error) {
	data, err := os.ReadFile(s.currentPath())
	if errors.Is(err, fs.ErrNotExist) {
		return "", ErrNoReview
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func (s Store) LoadCurrent() (*Review, error) {
	id, err := s.Current()
	if err != nil {
		return nil, err
	}
	return s.Load(id)
}

func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
