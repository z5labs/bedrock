// Copyright (c) 2025 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package lifecycle provides helpers for defining actions to execute relative to a [bedrock.App]s execution.
package lifecycle

import (
	"context"
	"errors"
)

// Hook represents functionality that needs to be performed
// at a specific "time" relative to the execution of [bedrock.App.Run].
type Hook interface {
	Run(context.Context) error
}

// HookFunc is a func variant of the [Hook] interface.
type HookFunc func(context.Context) error

// Run implements the [Hook] interface.
func (f HookFunc) Run(ctx context.Context) error {
	return f(ctx)
}

type multiHook []Hook

func (mh multiHook) Run(ctx context.Context) error {
	errs := make([]error, 0, len(mh))
	for _, h := range mh {
		err := h.Run(ctx)
		if err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	if len(errs) == 1 {
		return errs[0]
	}
	return errors.Join(errs...)
}

// MultiHook returns a [Hook] that's the logical concatenation
// of the provided [Hook]s. They're applied sequentially.
func MultiHook(hooks ...Hook) Hook {
	return multiHook(hooks)
}

// Context allows users to set actions which should be performed relative
// to the [bedrock.App.Run] execution.
type Context struct {
	PostRun Hook
}

type key struct{}

var contextKey = &key{}

// NewContext returns a new [context.Context] containing the lifecycle [Context].
func NewContext(parent context.Context, c *Context) context.Context {
	return context.WithValue(parent, contextKey, c)
}

// FromContext tries to extract a lifecycle [Context] from the given [context.Context].
func FromContext(ctx context.Context) (*Context, bool) {
	lc, ok := ctx.Value(contextKey).(*Context)
	return lc, ok
}
