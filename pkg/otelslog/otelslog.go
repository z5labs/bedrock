// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package otelslog provides a OpenTelemetry aware slog.Handler implementation.
package otelslog

import (
	"context"
	"log/slog"

	"github.com/z5labs/bedrock/pkg/slogfield"

	"go.opentelemetry.io/otel/trace"
)

// Handler is an slog.Handler which helps standardize and correlate your
// logs by automatically adding the Trace ID and Span ID to your logs.
type Handler struct {
	slog slog.Handler
}

// NewHandler
func NewHandler(h slog.Handler) *Handler {
	return &Handler{slog: h}
}

// New provides a simple wrapper for slog.New(NewHandler(h)).
func New(h slog.Handler) *slog.Logger {
	return slog.New(NewHandler(h))
}

// Enabled implements the slog.Handler interface.
func (h *Handler) Enabled(ctx context.Context, lvl slog.Level) bool {
	return h.slog.Enabled(ctx, lvl)
}

// Handle implements the slog.Handler interface.
func (h *Handler) Handle(ctx context.Context, record slog.Record) error {
	spanCtx := trace.SpanContextFromContext(ctx)
	if !spanCtx.IsValid() {
		return h.slog.Handle(ctx, record)
	}

	r := record.Clone()
	r.AddAttrs(
		slog.Group(
			"otel",
			slogfield.String("trace_id", spanCtx.TraceID().String()),
			slogfield.String("span_id", spanCtx.SpanID().String()),
		),
	)
	return h.slog.Handle(ctx, r)
}

// WithAttrs implements the slog.Handler interface.
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return NewHandler(h.slog.WithAttrs(attrs))
}

// WithGroup implements the slog.Handler interface.
func (h *Handler) WithGroup(name string) slog.Handler {
	return NewHandler(h.slog.WithGroup(name))
}
