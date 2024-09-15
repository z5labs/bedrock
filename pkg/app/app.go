// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package app provides helpers for common bedrock.App implementation patterns.
package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"

	"github.com/z5labs/bedrock"
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
		defer errRecover(&err)

		return app.Run(ctx)
	})
}

// PanicError
type PanicError struct {
	Value any
}

// Error implements the [error] interface.
func (e PanicError) Error() string {
	return fmt.Sprintf("recovered from panic: %v", e.Value)
}

func errRecover(err *error) {
	r := recover()
	if r == nil {
		return
	}

	rerr, ok := r.(error)
	if ok {
		*err = rerr
		return
	}
	*err = PanicError{Value: r}
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
	// PostRun is always executed regardless if the underlying [bedrock.App]
	// returns an error or panics.
	PostRun LifecycleHook
}

// WithLifecycleHooks wraps a given [bedrock.App] in an implementation
// that runs [LifecycleHook]s around the execution of app.Run.
func WithLifecycleHooks(app bedrock.App, lifecycle Lifecycle) bedrock.App {
	return runFunc(func(ctx context.Context) (err error) {
		// Always run PostRun hook regardless if app returns an error or panics.
		defer runPostRunHook(ctx, lifecycle.PostRun, &err)

		return app.Run(ctx)
	})
}

func runPostRunHook(ctx context.Context, hook LifecycleHook, err *error) {
	if hook == nil {
		return
	}

	hookErr := hook.Run(ctx)

	// errors.Join will not return an error if both
	// *err and hookErr are nil.
	*err = errors.Join(*err, hookErr)
}
