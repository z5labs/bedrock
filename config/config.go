// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package config

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"

	"github.com/z5labs/bedrock"
)

// ErrNotFound is the error of a configuration value that is not set.
//
// It is the only not-found error in the package: a missing environment
// variable, a zero [Value] and any [Source] that reports a value as absent
// all satisfy errors.Is(err, ErrNotFound). [Value.Or] and [Value.Default]
// recover from it and from nothing else, so a value that is present but
// malformed is never replaced by a fallback.
var ErrNotFound = errors.New("not found")

// Error is the error of reading a named configuration value. It renders as
// "config <Name>: <Err>", such as "config PORT: not found".
//
// Every error a [Value] returns is wrapped in an *Error exactly once, under
// the name of the value it was read from, so errors.AsType[*Error] finds
// the name and errors.Is still finds the cause.
type Error struct {
	// Name is the name of the value that failed, such as the name of an
	// environment variable. It is empty for a value that was never named,
	// such as one built by [Static].
	Name string

	// Err is the cause of the failure.
	Err error
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e.Name == "" {
		return "config: " + e.Err.Error()
	}
	return "config " + e.Name + ": " + e.Err.Error()
}

// Unwrap returns the cause of the failure.
func (e *Error) Unwrap() error {
	return e.Err
}

// wrap wraps err in an *Error named name, unless an *Error already wraps
// it, so that an error is named once, by the value that first read it.
func wrap(name string, err error) error {
	if _, ok := errors.AsType[*Error](err); ok {
		return err
	}
	return &Error{Name: name, Err: err}
}

// Source is anything a configuration value of type T can be read from.
//
// It is the extension point of the package. It is satisfied structurally,
// so a package that provides a source does not need to import config. Wrap
// a Source in a [Value] with [From] to give it a name and the combinators.
//
// A Source reports a value that is not set by returning an error that
// satisfies errors.Is(err, ErrNotFound).
type Source[T any] interface {
	Read(ctx context.Context) (T, error)
}

// Value is a named configuration value of type T, composed by chaining its
// methods:
//
//	port := config.Env(p.Environment, "PORT").Parse(strconv.Atoi).Default(8080)
//	n, err := port.Read(ctx)
//
// A Value is a description of how to read the value, not the value itself:
// nothing is read until [Value.Read] is called, and every call reads its
// sources again.
//
// Value is a concrete type rather than an interface because its
// combinators are generic methods, which an interface cannot declare. It
// satisfies [Source].
//
// The zero Value is not set: reading it returns [ErrNotFound]. It is the
// natural start of a chain of [Value.Or].
type Value[T any] struct {
	name string
	read func(context.Context) (T, error)
}

var _ Source[int] = Value[int]{}

// Read reads the value. It returns an error that satisfies
// errors.Is(err, ErrNotFound) when the value is not set, and the zero
// Value returns ErrNotFound itself. Every other error is wrapped in an
// [*Error] naming the value.
func (v Value[T]) Read(ctx context.Context) (T, error) {
	if v.read == nil {
		var zero T
		return zero, ErrNotFound
	}
	t, err := v.read(ctx)
	if err != nil {
		var zero T
		return zero, wrap(v.name, err)
	}
	return t, nil
}

// Map returns a Value that reads v and transforms it with f, which cannot
// fail. Use [Value.Parse] for a transformation that can. The returned Value
// has the name of v. Map panics if f is nil.
func (v Value[T]) Map[U any](f func(T) U) Value[U] {
	if f == nil {
		panic("config: Map requires a non-nil function")
	}
	return Value[U]{
		name: v.name,
		read: func(ctx context.Context) (U, error) {
			t, err := v.Read(ctx)
			if err != nil {
				var zero U
				return zero, err
			}
			return f(t), nil
		},
	}
}

// Parse returns a Value that reads v and transforms it with f, such as
// strconv.Atoi. An error from f is a failure of the value and is wrapped in
// an [*Error] with the name of v, so "PORT=abc" read through
// Parse(strconv.Atoi) fails with "config PORT: strconv.Atoi: ...". An empty
// string is a value like any other: f decides whether it is valid. Parse
// panics if f is nil.
func (v Value[T]) Parse[U any](f func(T) (U, error)) Value[U] {
	if f == nil {
		panic("config: Parse requires a non-nil function")
	}
	return Value[U]{
		name: v.name,
		read: func(ctx context.Context) (U, error) {
			t, err := v.Read(ctx)
			if err != nil {
				var zero U
				return zero, err
			}
			return f(t)
		},
	}
}

// FlatMap returns a Value that reads v, passes it to f, and reads the Value
// that f returns, such as the variable whose name v holds. An error from
// the returned Value keeps its own name; if that Value is unnamed, its
// errors are named after v. FlatMap panics if f is nil.
func (v Value[T]) FlatMap[U any](f func(T) Value[U]) Value[U] {
	if f == nil {
		panic("config: FlatMap requires a non-nil function")
	}
	return Value[U]{
		name: v.name,
		read: func(ctx context.Context) (U, error) {
			t, err := v.Read(ctx)
			if err != nil {
				var zero U
				return zero, err
			}
			return f(t).Read(ctx)
		},
	}
}

// Or returns a Value that reads v and, only when v is not set, reads next
// instead. Any other error from v is returned as it is, so a value that is
// present but malformed is never replaced by next. An error from next that
// does not already name its value is named after v; wrap next with [From]
// to give it a name of its own. Or panics if next is nil.
func (v Value[T]) Or(next Source[T]) Value[T] {
	if next == nil {
		panic("config: Or requires a non-nil Source")
	}
	return Value[T]{
		name: v.name,
		read: func(ctx context.Context) (T, error) {
			t, err := v.Read(ctx)
			if errors.Is(err, ErrNotFound) {
				return next.Read(ctx)
			}
			return t, err
		},
	}
}

// Default returns a Value that reads v and, only when v is not set, returns
// d instead. Any other error from v is returned as it is, so "PORT=abc"
// read through Parse(strconv.Atoi).Default(8080) is an error, never 8080.
func (v Value[T]) Default(d T) Value[T] {
	return v.Or(Static(d))
}

// Env returns a Value that reads the named variable from env.
//
// Every [Value.Read] calls env.Value(name) again; nothing is cached. A
// variable that is not set, reported by env as a [bedrock.ValueNotFoundError],
// is an error that satisfies both errors.Is(err, ErrNotFound) and
// errors.As(err, &bedrock.ValueNotFoundError{}). Any other error from env is
// a failure that [Value.Or] and [Value.Default] do not recover from. A
// variable that is set to the empty string is set: its value is "".
//
// Env panics if env is nil. In an App, pass p.Environment.
func Env(env bedrock.Environment, name string) Value[string] {
	if env == nil {
		panic("config: Env requires a non-nil bedrock.Environment")
	}
	return Value[string]{
		name: name,
		read: func(context.Context) (string, error) {
			s, err := env.Value(name)
			if _, ok := errors.AsType[bedrock.ValueNotFoundError](err); ok {
				return "", notFoundError{cause: err}
			}
			return s, err
		},
	}
}

// notFoundError reports a value that is not set, as ErrNotFound, while
// keeping the error of the source that reported it.
type notFoundError struct {
	cause error
}

// Error implements the error interface. It renders as ErrNotFound, so that
// a missing value renders the same whatever source reported it.
func (notFoundError) Error() string {
	return ErrNotFound.Error()
}

// Unwrap returns ErrNotFound and the error of the source.
func (e notFoundError) Unwrap() []error {
	return []error{ErrNotFound, e.cause}
}

// Func returns a Value named name that reads itself by calling fn. fn
// reports a value that is not set by returning an error that satisfies
// errors.Is(err, ErrNotFound). Func panics if fn is nil.
func Func[T any](name string, fn func(context.Context) (T, error)) Value[T] {
	if fn == nil {
		panic("config: Func requires a non-nil function")
	}
	return Value[T]{name: name, read: fn}
}

// From returns a Value named name that reads src. It is how a [Source]
// from outside the package gains a name and the combinators of [Value].
// From panics if src is nil.
func From[T any](name string, src Source[T]) Value[T] {
	if src == nil {
		panic("config: From requires a non-nil Source")
	}
	return Value[T]{name: name, read: src.Read}
}

// Static returns an unnamed Value that is always set to v.
func Static[T any](v T) Value[T] {
	return Value[T]{
		read: func(context.Context) (T, error) {
			return v, nil
		},
	}
}

// ParseInt64 parses s as a base 10 int64. It is a parser for
// [Value.Parse].
func ParseInt64(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}

// ParseFloat64 parses s as a float64. It is a parser for [Value.Parse].
func ParseFloat64(s string) (float64, error) {
	return strconv.ParseFloat(s, 64)
}

// OneOf returns a parser for [Value.Parse] that accepts a value only if it
// is one of allowed, and returns it unchanged:
//
//	level := config.Env(p.Environment, "LOG_LEVEL").Parse(config.OneOf("debug", "info", "warn", "error"))
//
// OneOf panics if allowed is empty, because no value could ever pass.
func OneOf[T comparable](allowed ...T) func(T) (T, error) {
	if len(allowed) == 0 {
		panic("config: OneOf requires at least one allowed value")
	}
	allowed = slices.Clone(allowed)
	return func(v T) (T, error) {
		if slices.Contains(allowed, v) {
			return v, nil
		}
		var zero T
		return zero, fmt.Errorf("%v is not one of %v", v, allowed)
	}
}
