// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package queue provides multiple patterns which implements the app.Runtime interface.
package queue

import (
	"context"
	"errors"
	"log/slog"

	"github.com/z5labs/app/pkg/slogfield"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"golang.org/x/sync/errgroup"
)

// Consumer
type Consumer[T any] interface {
	Consume(context.Context) (T, error)
}

// Processor
type Processor[T any] interface {
	Process(context.Context, T) error
}

type sequentialOptions struct {
	commonOptions
}

// SequentialOption
type SequentialOption interface {
	Option
	applySequential(*sequentialOptions)
}

func (f commonOptionFunc) applySequential(so *sequentialOptions) {
	f(&so.commonOptions)
}

// SequentialRuntime
type SequentialRuntime[T any] struct {
	log *slog.Logger
	c   Consumer[T]
	p   Processor[T]
}

// Sequential
func Sequential[T any](c Consumer[T], p Processor[T], opts ...Option) *SequentialRuntime[T] {
	so := &sequentialOptions{
		commonOptions: commonOptions{
			logHandler: noopLogHandler{},
		},
	}
	for _, opt := range opts {
		switch x := opt.(type) {
		case SequentialOption:
			x.applySequential(so)
		default:
			x.apply(so)
		}
	}

	return &SequentialRuntime[T]{
		log: slog.New(so.logHandler),
		c:   c,
		p:   p,
	}
}

// Run implements the app.Runtime interface.
func (rt *SequentialRuntime[T]) Run(ctx context.Context) error {
	tracer := otel.Tracer("queue")
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		spanCtx, span := tracer.Start(ctx, "SequentialRuntime.Run")
		item, err := consume(spanCtx, rt.c)
		if err != nil {
			rt.log.ErrorContext(spanCtx, "failed to consume", slogfield.Error(err))
			span.End()
			continue
		}

		select {
		case <-ctx.Done():
			span.End()
			return nil
		default:
		}

		err = process(spanCtx, rt.p, item.value)
		if err != nil {
			rt.log.ErrorContext(spanCtx, "failed to process", slogfield.Error(err))
		}
		span.End()
	}
}

type pipeOptions struct {
	commonOptions

	maxConcurrentProcessors int
}

// PipeOption
type PipeOption interface {
	Option
	applyPipe(*pipeOptions)
}

type pipeOptionFunc func(*pipeOptions)

func (f pipeOptionFunc) apply(v any) {
	po := v.(*pipeOptions)
	f(po)
}

func (f pipeOptionFunc) applyPipe(po *pipeOptions) {
	f(po)
}

func (f commonOptionFunc) applyPipe(po *pipeOptions) {
	f(&po.commonOptions)
}

// MaxConcurrentProcessors
func MaxConcurrentProcessors(n uint) PipeOption {
	return pipeOptionFunc(func(po *pipeOptions) {
		if n == 0 {
			return
		}
		po.maxConcurrentProcessors = int(n)
	})
}

// PipeRuntime
type PipeRuntime[T any] struct {
	log *slog.Logger
	c   Consumer[T]
	p   Processor[T]

	propagator              propagation.TextMapPropagator
	maxConcurrentProcessors int
}

// Pipe
func Pipe[T any](c Consumer[T], p Processor[T], opts ...Option) *PipeRuntime[T] {
	po := &pipeOptions{
		commonOptions: commonOptions{
			logHandler: noopLogHandler{},
		},
		maxConcurrentProcessors: -1,
	}
	for _, opt := range opts {
		switch x := opt.(type) {
		case PipeOption:
			x.applyPipe(po)
		default:
			x.apply(po)
		}
	}

	return &PipeRuntime[T]{
		log:                     slog.New(po.logHandler),
		c:                       c,
		p:                       p,
		propagator:              propagation.Baggage{},
		maxConcurrentProcessors: po.maxConcurrentProcessors,
	}
}

// Run implements the app.Runtime interface
func (rt *PipeRuntime[T]) Run(ctx context.Context) error {
	itemCh := make(chan *item[T])

	g, gctx := errgroup.WithContext(ctx)
	g.Go(rt.consumeItems(gctx, itemCh))
	g.Go(rt.processItems(gctx, itemCh))
	return g.Wait()
}

type item[T any] struct {
	value T

	// for concurrent Consumer-Processor implemetations
	// the otel context needs to be propagated between goroutines
	carrier propagation.TextMapCarrier
}

func (rt *PipeRuntime[T]) consumeItems(ctx context.Context, itemCh chan<- *item[T]) func() error {
	return func() error {
		defer close(itemCh)

		tracer := otel.Tracer("queue")
		for {
			spanCtx, span := tracer.Start(ctx, "PipeRuntime.consumeItems")

			select {
			case <-spanCtx.Done():
				span.End()
				return nil
			default:
			}

			item, err := consume(spanCtx, rt.c)
			if err != nil {
				rt.log.ErrorContext(spanCtx, "failed to consume", slogfield.Error(err))
				span.End()
				continue
			}

			item.carrier = make(propagation.MapCarrier)
			rt.propagator.Inject(spanCtx, item.carrier)

			select {
			case <-spanCtx.Done():
				span.End()
				return nil
			case itemCh <- item:
				span.End()
			}
		}
	}
}

func (rt *PipeRuntime[T]) processItems(ctx context.Context, itemCh <-chan *item[T]) func() error {
	return func() error {
		g, gctx := errgroup.WithContext(ctx)
		g.SetLimit(rt.maxConcurrentProcessors)

		for {
			var i *item[T]
			select {
			case <-gctx.Done():
				return g.Wait()
			case i = <-itemCh:
			}
			if i == nil {
				rt.log.Debug("stopping item processing since item channel was closed")
				return g.Wait()
			}

			propCtx := rt.propagator.Extract(gctx, i.carrier)
			g.Go(rt.processItem(propCtx, i))
		}
	}
}

func (rt *PipeRuntime[T]) processItem(ctx context.Context, i *item[T]) func() error {
	return func() error {
		spanCtx, span := otel.Tracer("queue").Start(ctx, "processItem")
		defer span.End()

		err := process(spanCtx, rt.p, i.value)
		if err != nil {
			rt.log.ErrorContext(spanCtx, "failed to process", slogfield.Error(err))
		}
		return nil
	}
}

func consume[T any](ctx context.Context, c Consumer[T]) (i *item[T], err error) {
	spanCtx, span := otel.Tracer("queue").Start(ctx, "consume")
	defer span.End()
	defer errRecover(&err)

	v, err := c.Consume(spanCtx)
	if err != nil {
		return nil, err
	}
	return &item[T]{value: v}, nil
}

func process[T any](ctx context.Context, p Processor[T], value T) (err error) {
	spanCtx, span := otel.Tracer("queue").Start(ctx, "process")
	defer span.End()
	defer errRecover(&err)

	return p.Process(spanCtx, value)
}

func errRecover(err *error) {
	r := recover()
	if r == nil {
		return
	}
	rerr, ok := r.(error)
	if !ok {
		*err = errors.New("recovered from consumer panic")
		return
	}
	*err = rerr
}

type noopLogHandler struct{}

func (noopLogHandler) Enabled(_ context.Context, _ slog.Level) bool  { return true }
func (noopLogHandler) Handle(_ context.Context, _ slog.Record) error { return nil }
func (h noopLogHandler) WithAttrs(_ []slog.Attr) slog.Handler        { return h }
func (h noopLogHandler) WithGroup(name string) slog.Handler          { return h }
