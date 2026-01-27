// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package bedrock

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
)

// Builder is a generic interface for building application components.
type Builder[T any] interface {
	Build(context.Context) (T, error)
}

// BuilderFunc is a function type that implements the Builder interface.
type BuilderFunc[T any] func(context.Context) (T, error)

// Build implements the [Builder] interface for BuilderFunc.
func (f BuilderFunc[T]) Build(ctx context.Context) (T, error) {
	return f(ctx)
}

// BuilderOf creates a Builder that always returns the provided value.
func BuilderOf[T any](value T) Builder[T] {
	return BuilderFunc[T](func(ctx context.Context) (T, error) {
		return value, nil
	})
}

// MustBuild builds the application component using the provided Builder.
func MustBuild[T any](ctx context.Context, builder Builder[T]) T {
	value, err := builder.Build(ctx)
	if err != nil {
		panic(err)
	}
	return value
}

// MemoizeBuilder wraps a Builder to cache its result after the first build.
// It is safe for concurrent use.
func MemoizeBuilder[T any](builder Builder[T]) Builder[T] {
	var (
		cachedValue T
		cachedErr   error
		once        sync.Once
	)
	return BuilderFunc[T](func(ctx context.Context) (T, error) {
		once.Do(func() {
			cachedValue, cachedErr = builder.Build(ctx)
		})
		return cachedValue, cachedErr
	})
}

// Map transforms the output of a Builder using the provided mapper function.
func Map[A, B any](builder Builder[A], mapper func(context.Context, A) (B, error)) Builder[B] {
	return BuilderFunc[B](func(ctx context.Context) (B, error) {
		appA, err := builder.Build(ctx)
		if err != nil {
			var zero B
			return zero, err
		}
		return mapper(ctx, appA)
	})
}

// Bind chains two Builders together, where the output of the first is used to create the second.
func Bind[A, B any](builder Builder[A], binder func(context.Context, A) Builder[B]) Builder[B] {
	return BuilderFunc[B](func(ctx context.Context) (B, error) {
		appA, err := builder.Build(ctx)
		if err != nil {
			var zero B
			return zero, err
		}
		return binder(ctx, appA).Build(ctx)
	})
}

// Runtime is an interface representing a runnable application component.
type Runtime interface {
	Run(context.Context) error
}

// RuntimeFunc is a function type that implements the Runtime interface.
type RuntimeFunc func(context.Context) error

// Run implements the [Runtime] interface for RuntimeFunc.
func (f RuntimeFunc) Run(ctx context.Context) error {
	return f(ctx)
}

// Runner is a generic interface for running application components.
type Runner[T Runtime] interface {
	Run(context.Context, Builder[T]) error
}

// RunnerFunc is a function type that implements the Runner interface.
type RunnerFunc[T Runtime] func(context.Context, Builder[T]) error

// Run implements the [Runner] interface for RunnerFunc.
func (f RunnerFunc[T]) Run(ctx context.Context, builder Builder[T]) error {
	return f(ctx, builder)
}

// DefaultRunner returns a Runner that builds and runs the application component.
func DefaultRunner[T Runtime]() Runner[T] {
	return RunnerFunc[T](func(ctx context.Context, builder Builder[T]) error {
		app, err := builder.Build(ctx)
		if err != nil {
			return err
		}
		return app.Run(ctx)
	})
}

// NotifyOnSignal wraps a Runner to listen for specified OS signals and cancel the context when received.
func NotifyOnSignal[T Runtime](runner Runner[T], signals ...os.Signal) Runner[T] {
	return RunnerFunc[T](func(ctx context.Context, builder Builder[T]) error {
		ctx, cancel := signal.NotifyContext(ctx, signals...)
		defer cancel()

		return runner.Run(ctx, builder)
	})
}

// RecoverPanics wraps a Runner to recover from panics during execution and return them as errors.
func RecoverPanics[T Runtime](runner Runner[T]) Runner[T] {
	return RunnerFunc[T](func(ctx context.Context, builder Builder[T]) (err error) {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("recovered from panic: %v", r)
			}
		}()
		return runner.Run(ctx, builder)
	})
}
