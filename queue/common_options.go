// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package queue

import (
	"log/slog"

	"github.com/z5labs/app/pkg/otelslog"
)

type commonOptions struct {
	logHandler slog.Handler
}

// Option
type Option interface {
	apply(any)
}

// CommonOption
type CommonOption interface {
	Option
	applyCommon(*commonOptions)
}

type commonOptionFunc func(*commonOptions)

func (f commonOptionFunc) apply(v any) {
	co := v.(*commonOptions)
	f(co)
}

func (f commonOptionFunc) applyCommon(co *commonOptions) {
	f(co)
}

// LogHandler
func LogHandler(h slog.Handler) CommonOption {
	return commonOptionFunc(func(co *commonOptions) {
		co.logHandler = otelslog.NewHandler(h)
	})
}
