package view

import (
	"context"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fsnotify/fsnotify"

	"github.com/aplotnikov/guided-review/internal/gitx"
	"github.com/aplotnikov/guided-review/internal/state"
)

func Run(ctx context.Context, store state.Store, repo gitx.Repo) error {
	if err := os.MkdirAll(store.Dir, 0o755); err != nil {
		return err
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer w.Close()
	if err := watchTree(w, store.Dir); err != nil {
		return err
	}
	p := tea.NewProgram(newModel(ctx, store, repo), tea.WithAltScreen(), tea.WithMouseCellMotion(), tea.WithContext(ctx))
	go forward(w, p)
	_, err = p.Run()
	if ctx.Err() != nil {
		return nil
	}
	return err
}

func watchTree(w *fsnotify.Watcher, dir string) error {
	if err := w.Add(dir); err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			if err := w.Add(filepath.Join(dir, e.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

func forward(w *fsnotify.Watcher, p *tea.Program) {
	for {
		select {
		case ev, ok := <-w.Events:
			if !ok {
				return
			}
			if ev.Has(fsnotify.Create) {
				if fi, err := os.Stat(ev.Name); err == nil && fi.IsDir() {
					_ = w.Add(ev.Name)
				}
			}
			if name := filepath.Base(ev.Name); name == state.FileName || state.IsCurrentFile(name) {
				p.Send(reloadMsg{})
			}
		case _, ok := <-w.Errors:
			if !ok {
				return
			}
		}
	}
}
