// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package config

import (
	"context"
	"errors"
	"sync"
)

// Struct returns a Value that reads a struct, or any other S, by calling
// build with a [Resolver] and reading each value build asks for through
// [Resolver.Get]:
//
//	cfg := config.Struct(func(r *config.Resolver) Settings {
//		return Settings{
//			Addr: r.Get(config.Env(p.Environment, "ADDR")),
//			Port: r.Get(config.Env(p.Environment, "PORT").Parse(strconv.Atoi).Default(8080)),
//		}
//	})
//	settings, err := cfg.Read(ctx)
//
// Every Get is attempted, even after one has failed, so a misconfigured
// service reports all of its problems in one error rather than one per
// restart. The failures are combined with errors.Join, so errors.Is and
// errors.AsType[*Error] find each of them, and the error renders one
// "config <Name>: <cause>" line per failed value. When any Get failed,
// [Value.Read] returns the zero S, never the partly built one.
//
// A failed Get returns the zero T, so build cannot use the result of one Get
// to decide what to read next: it would be deciding on a zero value. A read
// that depends on another value's result belongs in [Value.FlatMap], whose
// Value is then passed to Get.
//
// The returned Value is unnamed: each failure is already named after the
// value it was read from. Every [Value.Read] calls build again with a fresh
// Resolver. Struct panics if build is nil.
func Struct[S any](build func(*Resolver) S) Value[S] {
	if build == nil {
		panic("config: Struct requires a non-nil function")
	}
	return Value[S]{
		read: func(ctx context.Context) (S, error) {
			r := &Resolver{ctx: ctx}
			s := build(r)
			err := r.finish()
			if err != nil {
				var zero S
				return zero, err
			}
			return s, nil
		},
	}
}

// Resolver reads the values a [Struct] is built from. It is created by
// Struct for a single call of its build function and is valid only until
// that call returns.
//
// A Resolver is safe for concurrent use, so build may read its values from
// several goroutines, provided they all finish before build returns.
type Resolver struct {
	ctx context.Context

	mu   sync.Mutex
	done bool
	errs []error
}

// Get reads src and returns its value. When the read fails, Get records the
// error for [Struct] to return and returns the zero T, so build carries on
// and every value it reads is attempted.
//
// Get panics if src is nil, or if it is called after the build function of
// the Struct that created r has returned, since that error could no longer
// be reported.
func (r *Resolver) Get[T any](src Source[T]) T {
	if src == nil {
		panic("config: Get requires a non-nil Source")
	}
	t, err := src.Read(r.ctx)

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.done {
		panic("config: Get called after the Struct build function returned")
	}
	if err != nil {
		r.errs = append(r.errs, wrap("", err))
		var zero T
		return zero
	}
	return t
}

// finish ends the build and returns every error Get recorded, joined, or
// nil when there were none.
func (r *Resolver) finish() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.done = true
	return errors.Join(r.errs...)
}
