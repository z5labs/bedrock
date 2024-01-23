// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package noop

import (
	"context"
	"log/slog"
)

// LogHandler is a no-op implementation of the slog.Handler interface.
type LogHandler struct{}

// Enabled implements the slog.Handler interface.
func (LogHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

// Handle implements the slog.Handler interface.
func (LogHandler) Handle(_ context.Context, _ slog.Record) error { return nil }

// WithAttrs implements the slog.Handler interface.
func (h LogHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }

// WithGroup implements the slog.Handler interface.
func (h LogHandler) WithGroup(name string) slog.Handler { return h }
