package main

import (
	"context"
	"errors"
)

func cmdPlan(context.Context, env, []string) error    { return errors.New("not implemented") }
func cmdStep(context.Context, env, []string) error    { return errors.New("not implemented") }
func cmdComment(context.Context, env, []string) error { return errors.New("not implemented") }
func cmdStatus(context.Context, env, []string) error  { return nil }
func cmdView(context.Context, env) error              { return errors.New("not implemented") }
