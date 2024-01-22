// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package queue

import (
	"log/slog"

	"github.com/z5labs/bedrock/pkg/otelslog"
)

type commonOptions struct {
	logHandler slog.Handler
}

// CommonOption are options which are common to all queue based runtimes.
type CommonOption interface {
	SequentialOption
	ConcurrentOption
}

type commonOptionFunc func(*commonOptions)

func (f commonOptionFunc) applySequential(so *sequentialOptions) {
	f(&so.commonOptions)
}

func (f commonOptionFunc) applyPipe(po *concurrentOptions) {
	f(&po.commonOptions)
}

// LogHandler configures the underlying slog.Handler.
func LogHandler(h slog.Handler) CommonOption {
	return commonOptionFunc(func(co *commonOptions) {
		co.logHandler = otelslog.NewHandler(h)
	})
}
