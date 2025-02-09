// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package app provides helpers for common bedrock.App implementation patterns.
package app

import (
	"context"
	"errors"
	"os"
	"os/signal"

	"github.com/z5labs/bedrock"
	"github.com/z5labs/bedrock/internal/try"
	"github.com/z5labs/bedrock/lifecycle"
)

type runFunc func(context.Context) error

func (f runFunc) Run(ctx context.Context) error {
	return f(ctx)
}

// Recover will wrap the give [bedrock.App] with panic recovery.
// If the recovered panic value implements [error] then it will
// be directly returned. If it does not implement [error] then a
// [PanicError] will be returned instead.
func Recover(app bedrock.App) bedrock.App {
	return runFunc(func(ctx context.Context) (err error) {
		defer try.Recover(&err)

		return app.Run(ctx)
	})
}

// InterruptOn wraps a given [bedrock.App] in an implementation
// that cancels the [context.Context] that's passed to app.Run if an [os.Signal]
// is received by the running process.
func InterruptOn(app bedrock.App, signals ...os.Signal) bedrock.App {
	return runFunc(func(ctx context.Context) error {
		sigCtx, cancel := signal.NotifyContext(ctx, signals...)
		defer cancel()

		return app.Run(sigCtx)
	})
}

// PostRun defers the execution of the given [lifecycle.Hook] until
// after the given [bedrock.App] returns from its Run method. Since
// the [lifecycle.Hook] execution is deferred it will always execute
// even if the [bedrock.App.Run] panics.
func PostRun(app bedrock.App, hook lifecycle.Hook) bedrock.App {
	return runFunc(func(ctx context.Context) (err error) {
		defer runPostHook(&err, ctx, hook)
		return app.Run(ctx)
	})
}

func runPostHook(err *error, ctx context.Context, hook lifecycle.Hook) {
	hookErr := hook.Run(ctx)
	if hookErr == nil {
		return
	}
	if *err == nil {
		*err = hookErr
		return
	}
	*err = errors.Join(*err, hookErr)
}
