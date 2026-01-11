// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package config

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"strconv"
	"time"
)

// Value represents a configuration value that may or may not be set.
type Value[T any] struct {
	val T
	set bool
}

// ValueOf creates a Value that is set to the given value.
func ValueOf[T any](v T) Value[T] {
	return Value[T]{val: v, set: true}
}

// Value returns the value and a boolean indicating if it is set.
func (v Value[T]) Value() (T, bool) {
	return v.val, v.set
}

// Reader is an interface for reading configuration values.
type Reader[T any] interface {
	Read(context.Context) (Value[T], error)
}

// ReaderFunc is a function type that implements the Reader interface.
type ReaderFunc[T any] func(context.Context) (Value[T], error)

// Read implements the [Reader] interface for ReaderFunc.
func (f ReaderFunc[T]) Read(ctx context.Context) (Value[T], error) {
	return f(ctx)
}

// EmptyReader creates a Reader that always returns no value.
func EmptyReader[T any]() Reader[T] {
	return ReaderFunc[T](func(ctx context.Context) (Value[T], error) {
		return Value[T]{}, nil
	})
}

// ReaderOf creates a Reader that always returns the given value.
func ReaderOf[T any](val T) Reader[T] {
	return ReaderFunc[T](func(ctx context.Context) (Value[T], error) {
		return ValueOf(val), nil
	})
}

// ErrValueNotSet is returned when a configuration value is not set.
var ErrValueNotSet = errors.New("config: value not set")

// Read reads the value from the given Reader.
func Read[T any](ctx context.Context, r Reader[T]) (T, error) {
	var zero T
	val, err := r.Read(ctx)
	if err != nil {
		return zero, err
	}
	if !val.set {
		return zero, ErrValueNotSet
	}
	return val.val, nil
}

// Must reads the value from the given Reader, panicking if the value is not set.
func Must[T any](ctx context.Context, r Reader[T]) T {
	val, err := Read(ctx, r)
	if err != nil {
		panic(err)
	}
	return val
}

// MustOr reads the value from the given Reader, returning the default value if the value is not set.
func MustOr[T any](ctx context.Context, def T, r Reader[T]) T {
	val, err := Read(ctx, r)
	if err != nil {
		return def
	}
	return val
}

// Default returns a Reader that provides a default value if the original Reader does not have a value set.
func Default[T any](defaultVal T, reader Reader[T]) Reader[T] {
	return ReaderFunc[T](func(ctx context.Context) (Value[T], error) {
		val, err := reader.Read(ctx)
		if err != nil {
			return Value[T]{}, err
		}

		if v, ok := val.Value(); ok {
			return ValueOf(v), nil
		}

		return ValueOf(defaultVal), nil
	})
}

// Or returns a Reader that tries multiple Readers in order, returning the first value that is set.
func Or[T any](readers ...Reader[T]) Reader[T] {
	return ReaderFunc[T](func(ctx context.Context) (Value[T], error) {
		for _, r := range readers {
			val, err := r.Read(ctx)
			if err != nil {
				return Value[T]{}, err
			}

			if v, ok := val.Value(); ok {
				return ValueOf(v), nil
			}
		}

		return Value[T]{}, nil
	})
}

// Map transforms the output of a Reader using the provided mapping function.
func Map[A, B any](reader Reader[A], mapper func(context.Context, A) (B, error)) Reader[B] {
	return ReaderFunc[B](func(ctx context.Context) (Value[B], error) {
		aVal, err := reader.Read(ctx)
		if err != nil {
			return Value[B]{}, err
		}

		a, ok := aVal.Value()
		if !ok {
			return Value[B]{}, nil
		}

		b, err := mapper(ctx, a)
		if err != nil {
			return Value[B]{}, err
		}

		return ValueOf(b), nil
	})
}

// Bind chains two Readers together, where the output of the first is used to create the second.
func Bind[A, B any](reader Reader[A], binder func(context.Context, A) Reader[B]) Reader[B] {
	return ReaderFunc[B](func(ctx context.Context) (Value[B], error) {
		aVal, err := reader.Read(ctx)
		if err != nil {
			return Value[B]{}, err
		}

		a, ok := aVal.Value()
		if !ok {
			return Value[B]{}, nil
		}

		return binder(ctx, a).Read(ctx)
	})
}

// Env returns a Reader that reads a string value from the environment variable with the given name.
func Env(name string) Reader[string] {
	return ReaderFunc[string](func(ctx context.Context) (Value[string], error) {
		val, ok := os.LookupEnv(name)
		if !ok {
			return Value[string]{}, nil
		}

		return ValueOf(val), nil
	})
}

// ReadFile returns a Reader that reads a file from the given path.
func ReadFile(path string) Reader[*os.File] {
	return ReaderFunc[*os.File](func(ctx context.Context) (Value[*os.File], error) {
		f, err := os.Open(path)
		if err != nil {
			if os.IsNotExist(err) {
				return Value[*os.File]{}, nil
			}
			return Value[*os.File]{}, err
		}

		return ValueOf(f), nil
	})
}

// BoolFromString returns a Reader that parses a boolean from a string Reader.
func BoolFromString(r Reader[string]) Reader[bool] {
	return Map(r, func(ctx context.Context, s string) (bool, error) {
		b, err := strconv.ParseBool(s)
		if err != nil {
			return false, err
		}

		return b, nil
	})
}

// IntFromString returns a Reader that parses an integer from a string Reader.
func IntFromString(r Reader[string]) Reader[int] {
	return Map(r, func(ctx context.Context, s string) (int, error) {
		n, err := strconv.Atoi(s)
		if err != nil {
			return 0, err
		}

		return n, nil
	})
}

// Int64FromBytes returns a Reader that reads an int64 from a byte stream using the specified endianness.
func Int64FromBytes[T io.Reader](endian binary.ByteOrder, r Reader[T]) Reader[int64] {
	return Map(r, func(ctx context.Context, t T) (int64, error) {
		var b [8]byte
		_, err := t.Read(b[:])
		if err != nil {
			return 0, err
		}

		n := int64(endian.Uint64(b[:]))
		return n, nil
	})
}

// Int64FromString returns a Reader that parses an int64 from a string Reader.
func Int64FromString(r Reader[string]) Reader[int64] {
	return Map(r, func(ctx context.Context, s string) (int64, error) {
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return 0, err
		}

		return n, nil
	})
}

// Float64FromString returns a Reader that parses a float64 from a string Reader.
func Float64FromString(r Reader[string]) Reader[float64] {
	return Map(r, func(ctx context.Context, s string) (float64, error) {
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0, err
		}

		return f, nil
	})
}

// DurationFromString returns a Reader that parses a time.Duration from a string Reader.
func DurationFromString(r Reader[string]) Reader[time.Duration] {
	return Map(r, func(ctx context.Context, s string) (time.Duration, error) {
		d, err := time.ParseDuration(s)
		if err != nil {
			return 0, err
		}

		return d, nil
	})
}
