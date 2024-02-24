// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package maskslog

import (
	"context"
	"log/slog"
	"sync"
)

type options struct {
	attrTransformers   map[string]func(slog.Attr) slog.Attr
	recordTransformers []func(slog.Record) slog.Record
}

// Option helps configure the Handler.
type Option interface {
	applyOption(*options)
}

type optionFunc func(*options)

func (f optionFunc) applyOption(opts *options) {
	f(opts)
}

// Message registers a function for masking slog.Record messages.
func Message(f func(string) string) Option {
	return optionFunc(func(o *options) {
		o.recordTransformers = append(o.recordTransformers, func(r slog.Record) slog.Record {
			r.Message = f(r.Message)
			return r
		})
	})
}

var attrPool = &sync.Pool{
	New: func() any {
		s := make([]slog.Attr, 0, 5)
		return &s
	},
}

// Attr registers a function for masking a slog.Attr given its key.
func Attr(key string, f func(slog.Attr) slog.Attr) Option {
	return optionFunc(func(o *options) {
		o.attrTransformers[key] = f
		o.recordTransformers = append(o.recordTransformers, func(r slog.Record) slog.Record {
			attrs, ok := attrPool.Get().(*[]slog.Attr)
			if !ok {
				*attrs = make([]slog.Attr, 0, 5)
			}
			defer func() {
				*attrs = (*attrs)[:0]
				attrPool.Put(attrs)
			}()

			r.Attrs(func(a slog.Attr) bool {
				if a.Key == key {
					a = f(a)
				}
				*attrs = append(*attrs, a)
				return true
			})

			nr := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
			nr.AddAttrs(*attrs...)
			return nr
		})
	})
}

// AnonymousStringAttr is a helper function for converting any slog.Attr
// into the anonymized string, "****". It completely ignores the given
// slog.Attr value type and always return a string value.
func AnonymousStringAttr(a slog.Attr) slog.Attr {
	return slog.String(a.Key, "****")
}

// Handler is an slog.Handler.
type Handler struct {
	slog slog.Handler

	attrTransformers   map[string]func(slog.Attr) slog.Attr
	recordTransformers []func(slog.Record) slog.Record
}

// NewHandler returns a new Handler.
func NewHandler(h slog.Handler, opts ...Option) *Handler {
	o := &options{
		attrTransformers: make(map[string]func(slog.Attr) slog.Attr),
	}
	for _, opt := range opts {
		opt.applyOption(o)
	}
	return &Handler{
		slog:               h,
		attrTransformers:   o.attrTransformers,
		recordTransformers: o.recordTransformers,
	}
}

// Enabled implements the slog.Handler interface.
func (h *Handler) Enabled(ctx context.Context, lvl slog.Level) bool {
	return h.slog.Enabled(ctx, lvl)
}

// Handle implements the slog.Handler interface.
func (h *Handler) Handle(ctx context.Context, record slog.Record) error {
	for _, t := range h.recordTransformers {
		record = t(record)
	}
	return h.slog.Handle(ctx, record)
}

// WithAttrs implements the slog.Handler interface.
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	nr := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		f, exists := h.attrTransformers[a.Key]
		if !exists {
			nr[i] = a
			continue
		}
		nr[i] = f(a)
	}
	return NewHandler(h.slog.WithAttrs(nr))
}

// WithGroup implements the slog.Handler interface.
func (h *Handler) WithGroup(name string) slog.Handler {
	nh := NewHandler(h.slog.WithGroup(name))
	nh.attrTransformers = h.attrTransformers
	nh.recordTransformers = h.recordTransformers
	return nh
}
