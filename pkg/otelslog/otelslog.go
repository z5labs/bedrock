// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package otelslog provides a OpenTelemetry aware slog.Handler implementation.
package otelslog

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Handler
type Handler struct {
	slog slog.Handler
}

// NewHandler
func NewHandler(h slog.Handler) *Handler {
	return &Handler{slog: h}
}

// Enabled
func (h *Handler) Enabled(ctx context.Context, lvl slog.Level) bool {
	return h.slog.Enabled(ctx, lvl)
}

// Handle
func (h *Handler) Handle(ctx context.Context, record slog.Record) error {
	span := trace.SpanFromContext(ctx)
	if span == nil {
		return h.slog.Handle(ctx, record)
	}
	if !span.IsRecording() {
		return h.slog.Handle(ctx, record)
	}

	attrs := make([]attribute.KeyValue, 0, record.NumAttrs())
	record.Attrs(func(attr slog.Attr) bool {
		attrs = appendKeyValue(attrs, attr.Key, attr.Value)
		return true
	})
	span.AddEvent("log", trace.WithAttributes(attrs...))

	if record.Level >= slog.LevelError {
		span.SetStatus(codes.Error, record.Message)
	}

	return h.slog.Handle(ctx, record)
}

// WithAttrs
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return NewHandler(h.slog.WithAttrs(attrs))
}

// WithGroup
func (h *Handler) WithGroup(name string) slog.Handler {
	return NewHandler(h.slog.WithGroup(name))
}

func appendKeyValue(attrs []attribute.KeyValue, key string, value slog.Value) []attribute.KeyValue {
	switch value.Kind() {
	case slog.KindAny:
		v := value.Any()
		switch x := v.(type) {
		case error:
			return append(attrs, attribute.String(key, x.Error()))
		case fmt.Stringer:
			return append(attrs, attribute.String(key, x.String()))
		default:
			// TODO: what to do?
			return attrs
		}
	case slog.KindBool:
		return append(attrs, attribute.Bool(key, value.Bool()))
	case slog.KindDuration:
		return append(attrs, attribute.Int64(key, value.Int64()))
	case slog.KindFloat64:
		return append(attrs, attribute.Float64(key, value.Float64()))
	case slog.KindGroup:
		as := value.Group()
		for _, a := range as {
			attrs = appendKeyValue(attrs, key+"."+a.Key, a.Value)
		}
		return attrs
	case slog.KindInt64:
		return append(attrs, attribute.Int64(key, value.Int64()))
	case slog.KindLogValuer:
		return appendKeyValue(attrs, key, value.LogValuer().LogValue())
	case slog.KindString:
		return append(attrs, attribute.String(key, value.String()))
	case slog.KindTime:
		return append(attrs, attribute.String(key, value.Time().Format(time.RFC3339)))
	case slog.KindUint64:
		return append(attrs, attribute.Int64(key, int64(value.Uint64())))
	default:
		return append(attrs, attribute.String(key+"_error", fmt.Sprintf("otelslog: unknown attribute type: %v", value)))
	}
}
