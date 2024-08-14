// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package bedrock

import (
	"context"
	"fmt"

	"github.com/z5labs/bedrock/pkg/config"
)

// App represents the entry point for user specific code.
type App interface {
	Run(context.Context) error
}

// AppBuilder represents anything which can initialize a Runtime.
type AppBuilder[T any] interface {
	Build(ctx context.Context, cfg T) (App, error)
}

// AppBuilderFunc is a functional implementation of
// the AppBuilder interface.
type AppBuilderFunc[T any] func(context.Context, T) (App, error)

// Build implements the RuntimeBuilder interface.
func (f AppBuilderFunc[T]) Build(ctx context.Context, cfg T) (App, error) {
	return f(ctx, cfg)
}

// Run executes the application. It's responsible for reading the provided
// config sources, unmarshalling them into the generic config type, using
// the config and builder to build the users [App] and, lastly, running the
// returned [App].
func Run[T any](ctx context.Context, builder AppBuilder[T], srcs ...config.Source) error {
	m, err := config.Read(srcs...)
	if err != nil {
		return ConfigReadError{Cause: err}
	}

	var cfg T
	err = m.Unmarshal(&cfg)
	if err != nil {
		return ConfigUnmarshalError{Cause: err}
	}

	app, err := builder.Build(ctx, cfg)
	if err != nil {
		return AppBuildError{Cause: err}
	}

	err = app.Run(ctx)
	if err != nil {
		return AppRunError{Cause: err}
	}
	return nil
}

// ConfigReadError
type ConfigReadError struct {
	Cause error
}

// Error implements the [builtin.error] interface.
func (e ConfigReadError) Error() string {
	return fmt.Sprintf("failed to read config source(s): %s", e.Cause)
}

// Unwrap implements the implicit interface used by [errors.Is] and [errors.As].
func (e ConfigReadError) Unwrap() error {
	return e.Cause
}

// ConfigUnmarshalError
type ConfigUnmarshalError struct {
	Cause error
}

// Error implements the [builtin.error] interface.
func (e ConfigUnmarshalError) Error() string {
	return fmt.Sprintf("failed to unmarshal read config source(s) into custom type: %s", e.Cause)
}

// Unwrap implements the implicit interface used by [errors.Is] and [errors.As].
func (e ConfigUnmarshalError) Unwrap() error {
	return e.Cause
}

// AppBuildError
type AppBuildError struct {
	Cause error
}

// Error implements the [builtin.error] interface.
func (e AppBuildError) Error() string {
	return fmt.Sprintf("failed to build app: %s", e.Cause)
}

// Unwrap implements the implicit interface used by [errors.Is] and [errors.As].
func (e AppBuildError) Unwrap() error {
	return e.Cause
}

// AppRunError
type AppRunError struct {
	Cause error
}

// Error implements the [builtin.error] interface.
func (e AppRunError) Error() string {
	return fmt.Sprintf("failed to run app: %s", e.Cause)
}

// Unwrap implements the implicit interface used by [errors.Is] and [errors.As].
func (e AppRunError) Unwrap() error {
	return e.Cause
}
