// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package app provides helpers for common bedrock.App implementation patterns.
package app

import (
	"context"
	"os"
	"os/signal"

	"github.com/z5labs/bedrock"
)

type runFunc func(context.Context) error

func (f runFunc) Run(ctx context.Context) error {
	return f(ctx)
}

// WithSignalNotifications wraps a given [bedrock.App] in an implementation
// that cancels the [context.Context] that's passed to app.Run if an [os.Signal]
// is received by the running process.
func WithSignalNotifications(app bedrock.App, signals ...os.Signal) bedrock.App {
	return runFunc(func(ctx context.Context) error {
		sigCtx, cancel := signal.NotifyContext(ctx, signals...)
		defer cancel()

		return app.Run(sigCtx)
	})
}

// LifecycleHook represents functionality that needs to be performed
// at a specific "time" relative to the execution of [bedrock.App.Run].
type LifecycleHook interface {
	Run(context.Context) error
}

// LifecycleHookFunc is a convenient helper type for implementing a [LifecycleHook]
// from just a regular func.
type LifecycleHookFunc func(context.Context) error

// Run implements the [LifecycleHook] interface.
func (f LifecycleHookFunc) Run(ctx context.Context) error {
	return f(ctx)
}

// Lifecycle
type Lifecycle struct {
	// PreRun is run before app.Run.
	PreRun LifecycleHook

	// PostRun is always executed regardless if the underlying [bedrock.App]
	// returns an error or panics.
	PostRun LifecycleHook
}

// WithLifecycleHooks wraps a given [bedrock.App] in an implementation
// that runs [LifecycleHook]s around the execution of app.Run.
func WithLifecycleHooks(app bedrock.App, lifecycle Lifecycle) bedrock.App {
	return runFunc(func(ctx context.Context) error {
		err := runHook(ctx, lifecycle.PreRun)
		if err != nil {
			return err
		}

		// Always run PostRun hook regardless if app returns an error or panics.
		defer func() {
			_ = runHook(ctx, lifecycle.PostRun)
		}()

		return app.Run(ctx)
	})
}

func runHook(ctx context.Context, hook LifecycleHook) error {
	if hook == nil {
		return nil
	}
	return hook.Run(ctx)
}
