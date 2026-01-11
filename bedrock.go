// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package bedrock provides a functional, composable framework for building and running applications.
//
// The package is built around three core abstractions:
//
//   - Builder[T]: A generic interface for constructing application components with context support
//   - Runtime: An interface representing a runnable application component
//   - Runner[T]: An interface for executing application components built from Builders
//
// # Functional Composition
//
// Bedrock embraces functional programming patterns to enable flexible composition:
//
//   - Map: Transform builder outputs using pure functions
//   - Bind: Chain builders together, allowing the output of one to inform the construction of the next
//
// These combinators allow you to build complex applications from simple, reusable components.
//
// # Basic Usage
//
// Create a builder for your application component:
//
//	builder := bedrock.BuilderFunc[MyApp](func(ctx context.Context) (MyApp, error) {
//	    return MyApp{}, nil
//	})
//
// Create a runtime by transforming the builder:
//
//	runtime := bedrock.Map(builder, func(app MyApp) (bedrock.Runtime, error) {
//	    return bedrock.RuntimeFunc(func(ctx context.Context) error {
//	        return app.Start(ctx)
//	    }), nil
//	})
//
// Run the application with signal handling and panic recovery:
//
//	runner := bedrock.RecoverPanics(
//	    bedrock.NotifyOnSignal(
//	        bedrock.DefaultRunner[bedrock.Runtime](),
//	        os.Interrupt,
//	    ),
//	)
//	if err := runner.Run(context.Background(), runtime); err != nil {
//	    log.Fatal(err)
//	}
package bedrock

import (
	"context"
	"fmt"
	"os"
	"os/signal"
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

// Map transforms the output of a Builder using the provided mapper function.
func Map[A, B any](builder Builder[A], mapper func(A) (B, error)) Builder[B] {
	return BuilderFunc[B](func(ctx context.Context) (B, error) {
		appA, err := builder.Build(ctx)
		if err != nil {
			var zero B
			return zero, err
		}
		return mapper(appA)
	})
}

// Bind chains two Builders together, where the output of the first is used to create the second.
func Bind[A, B any](builder Builder[A], binder func(A) Builder[B]) Builder[B] {
	return BuilderFunc[B](func(ctx context.Context) (B, error) {
		appA, err := builder.Build(ctx)
		if err != nil {
			var zero B
			return zero, err
		}
		return binder(appA).Build(ctx)
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
