// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package bedrock

import (
	"context"
	"errors"
	"fmt"

	"github.com/z5labs/bedrock/config"
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

// PanicError represents a value that was recovered from a panic.
type PanicError struct {
	Value any
}

// Error implements the [error] interface.
func (e PanicError) Error() string {
	return fmt.Sprintf("recovered from panic: %v", e.Value)
}

// Unwrap implements the interface used by [errors.Unwrap], [errors.Is] and [errors.As].
func (e PanicError) Unwrap() error {
	if e.Value == nil {
		return nil
	}
	if err, ok := e.Value.(error); ok {
		return err
	}
	return nil
}

// Recover calls [recover] and if a value is captured it will be wrapped
// into a [PanicError]. The [PanicError] will then be joined with any
// value, err, may reference. The joining is performed using [errors.Join].
func Recover(err *error) {
	r := recover()
	if r == nil {
		return
	}
	*err = errors.Join(*err, PanicError{
		Value: r,
	})
}

// Run executes the application. It's responsible for reading the provided
// config sources, unmarshalling them into the generic config type, using
// the config and builder to build the users [App] and, lastly, running the
// returned [App].
func Run[T any](ctx context.Context, builder AppBuilder[T], srcs ...config.Source) (err error) {
	defer Recover(&err)

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
