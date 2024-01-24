// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package queue

import (
	"context"
	"errors"
	"log/slog"

	"github.com/z5labs/bedrock/pkg/noop"
	"github.com/z5labs/bedrock/pkg/slogfield"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"golang.org/x/sync/errgroup"
)

// ErrNoItem should be returned by Consumers
// when no item has been consumed.
var ErrNoItem = errors.New("queue: no item")

// Consumer consumes items from a queue.
//
// If no item is consumed, then the Consumer
// should return ErrNoItem.
type Consumer[T any] interface {
	Consume(context.Context) (T, error)
}

// Processor processes items that are retrieved from a queue.
type Processor[T any] interface {
	Process(context.Context, T) error
}

type sequentialOptions struct {
	commonOptions
}

// SequentialOption are options for configuring the SequentialRuntime.
type SequentialOption interface {
	applySequential(*sequentialOptions)
}

// SequentialRuntime is a bedrock.Runtime for sequentially processing items from a queue.
type SequentialRuntime[T any] struct {
	log *slog.Logger
	c   Consumer[T]
	p   Processor[T]
}

// Sequential returns a fully initialized SequentialRuntime.
//
// Sequential will first consume an item from the Consumer, c. Then,
// process that item with the given Processor, p. After, processing
// the item, this sequence repeats. Thus, no new item will be consumed
// from the queue until the current item has been processed.
func Sequential[T any](c Consumer[T], p Processor[T], opts ...SequentialOption) *SequentialRuntime[T] {
	so := &sequentialOptions{
		commonOptions: commonOptions{
			logHandler: noop.LogHandler{},
		},
	}
	for _, opt := range opts {
		opt.applySequential(so)
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
		if errors.Is(err, ErrNoItem) {
			span.End()
			continue
		}
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

type concurrentOptions struct {
	commonOptions

	maxConcurrentProcessors int
}

// ConcurrentOption are options for configuring the ConcurrentRuntime.
type ConcurrentOption interface {
	applyPipe(*concurrentOptions)
}

type concurrentOptionFunc func(*concurrentOptions)

func (f concurrentOptionFunc) applyPipe(po *concurrentOptions) {
	f(po)
}

// MaxConcurrentProcessors configures a limit for the number
// of processor goroutines actively running.
func MaxConcurrentProcessors(n uint) ConcurrentOption {
	return concurrentOptionFunc(func(po *concurrentOptions) {
		if n == 0 {
			return
		}
		po.maxConcurrentProcessors = int(n)
	})
}

// ConcurrentRuntime is a bedrock.Runtime for concurrently processing items from a queue.
type ConcurrentRuntime[T any] struct {
	log *slog.Logger
	c   Consumer[T]
	p   Processor[T]

	propagator              propagation.TextMapPropagator
	maxConcurrentProcessors int
}

// Concurrent returns a fully initialized ConcurrentRuntime.
//
// Concurrent will consume and process items as concurrent processes.
// For every item returned by the Consumer, c, the Processor, p, is
// called in a separate goroutine to process the item. Due to the concurrent
// execution of the Consumer and Processor, new items will be consumed
// before the current item has been completely processed.
func Concurrent[T any](c Consumer[T], p Processor[T], opts ...ConcurrentOption) *ConcurrentRuntime[T] {
	po := &concurrentOptions{
		commonOptions: commonOptions{
			logHandler: noop.LogHandler{},
		},
		maxConcurrentProcessors: -1,
	}
	for _, opt := range opts {
		opt.applyPipe(po)
	}

	return &ConcurrentRuntime[T]{
		log:                     slog.New(po.logHandler),
		c:                       c,
		p:                       p,
		propagator:              propagation.TraceContext{},
		maxConcurrentProcessors: po.maxConcurrentProcessors,
	}
}

// Run implements the app.Runtime interface
func (rt *ConcurrentRuntime[T]) Run(ctx context.Context) error {
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

func (rt *ConcurrentRuntime[T]) consumeItems(ctx context.Context, itemCh chan<- *item[T]) func() error {
	return func() error {
		defer close(itemCh)

		tracer := otel.Tracer("queue")
		for {
			spanCtx, span := tracer.Start(ctx, "ConcurrentRuntime.consumeItems")

			select {
			case <-spanCtx.Done():
				span.End()
				return nil
			default:
			}

			item, err := consume(spanCtx, rt.c)
			if errors.Is(err, ErrNoItem) {
				span.End()
				continue
			}
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

func (rt *ConcurrentRuntime[T]) processItems(ctx context.Context, itemCh <-chan *item[T]) func() error {
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

func (rt *ConcurrentRuntime[T]) processItem(ctx context.Context, i *item[T]) func() error {
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
