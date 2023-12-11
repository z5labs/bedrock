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

// CommonOption
type CommonOption interface {
	SequentialOption
	PipeOption
}

type commonOptionFunc func(*commonOptions)

func (f commonOptionFunc) applySequential(so *sequentialOptions) {
	f(&so.commonOptions)
}

func (f commonOptionFunc) applyPipe(po *pipeOptions) {
	f(&po.commonOptions)
}

// LogHandler
func LogHandler(h slog.Handler) CommonOption {
	return commonOptionFunc(func(co *commonOptions) {
		co.logHandler = otelslog.NewHandler(h)
	})
}
