package main

import (
	"context"

	"github.com/aplotnikov/guided-review/internal/view"
)

func cmdView(ctx context.Context, e env) error {
	s, err := openSession(ctx, e.dir)
	if err != nil {
		return err
	}
	return view.Run(ctx, s.store, s.repo)
}
