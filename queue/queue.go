// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package queue

import (
	"context"
	"errors"
	"math"

	"github.com/uptrace/opentelemetry-go-extra/otelzap"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

// ErrEndOfItems
var ErrEndOfItems = errors.New("queue: end of items")

// Item
type Item[T any] struct {
	Value T
}

// Consumer
type Consumer[T any] interface {
	Consume(context.Context) (*Item[T], error)
}

// ConsumerFunc
type ConsumerFunc[T any] func(context.Context) (*Item[T], error)

// Consume implements the Consumer interface.
func (f ConsumerFunc[T]) Consume(ctx context.Context) (*Item[T], error) {
	return f(ctx)
}

// Processor
type Processor[T any] interface {
	Process(context.Context, T) error
}

// ProcessorFunc
type ProcessorFunc[T any] func(context.Context, T) error

// Process implements the Processor interface.
func (f ProcessorFunc[T]) Process(ctx context.Context, t T) error {
	return f(ctx, t)
}

type config struct {
	log                     *zap.Logger
	maxConcurrentProcessors int
	qps                     []func(context.Context, *Runtime) error
}

// Option
type Option func(*config)

// Logger
func Logger(logger *zap.Logger) Option {
	return func(cfg *config) {
		cfg.log = logger
	}
}

// MaxConcurrentProcessors
func MaxConcurrentProcessors(n int) Option {
	return func(cfg *config) {
		cfg.maxConcurrentProcessors = n
	}
}

// Pipe registers a consumer/processor pair with the queue runtime.
func Pipe[T any](c Consumer[T], p Processor[T]) Option {
	return func(cfg *config) {
		cfg.qps = append(cfg.qps, pipe(c, p))
	}
}

// Sequential
func Sequential[T any](c Consumer[T], p Processor[T]) Option {
	return func(cfg *config) {
		cfg.qps = append(cfg.qps, sequential(c, p))
	}
}

func queueProcessor(f func(context.Context, *Runtime) error) Option {
	return func(cfg *config) {
		cfg.qps = append(cfg.qps, f)
	}
}

// Runtime
type Runtime struct {
	log                     *otelzap.Logger
	qps                     []func(context.Context, *Runtime) error
	maxConcurrentProcessors int
}

// NewRuntime
func NewRuntime(opts ...Option) *Runtime {
	cfg := &config{
		log:                     zap.NewNop(),
		maxConcurrentProcessors: -1,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	r := &Runtime{
		log:                     otelzap.New(cfg.log),
		maxConcurrentProcessors: cfg.maxConcurrentProcessors,
		qps:                     cfg.qps,
	}
	return r
}

// Run implements the app.Runtime interface.
func (r *Runtime) Run(ctx context.Context) error {
	g, gctx := errgroup.WithContext(ctx)
	for _, qp := range r.qps {
		qp := qp
		g.Go(func() error {
			return qp(gctx, r)
		})
	}
	return g.Wait()
}

type itemWithId[T any] struct {
	id   int64
	item *Item[T]
}

func pipe[T any](c Consumer[T], p Processor[T]) func(context.Context, *Runtime) error {
	return func(ctx context.Context, rt *Runtime) error {
		itemCh := make(chan *itemWithId[T])
		g, gctx := errgroup.WithContext(ctx)
		g.Go(consumeQueue(gctx, rt, itemCh, c))
		g.Go(processItems(gctx, rt, itemCh, p))
		return g.Wait()
	}
}

func consumeQueue[T any](ctx context.Context, rt *Runtime, itemCh chan<- *itemWithId[T], c Consumer[T]) func() error {
	return func() error {
		defer close(itemCh)

		var itemIdx int64 = math.MinInt64
		tracer := otel.Tracer("queue")
		for {
			itemIdx += 1
			if itemIdx == math.MaxInt64 {
				itemIdx = math.MinInt64
			}
			spanCtx, span := tracer.Start(ctx, "consumeQueue", trace.WithAttributes(attribute.Int64("queue.item.index", itemIdx)))
			select {
			case <-spanCtx.Done():
				rt.log.Ctx(spanCtx).Info("context cancelled before item could be consumed")
				return nil
			default:
			}

			item, err := consume[T](spanCtx, c)
			if err == ErrEndOfItems {
				rt.log.Ctx(spanCtx).Info("end of queue")
				span.End()
				return nil
			}
			if err != nil {
				rt.log.Ctx(spanCtx).Error("encountered error when consuming item from queue", zap.Error(err))
				span.End()
				continue
			}
			if item == nil {
				rt.log.Ctx(spanCtx).Info("queue returned a nil item")
				span.End()
				continue
			}
			it := &itemWithId[T]{
				id:   itemIdx,
				item: item,
			}
			select {
			case <-spanCtx.Done():
				rt.log.Ctx(spanCtx).Warn("context was cancelled before item could be processed")
				span.End()
				return nil
			case itemCh <- it:
				rt.log.Ctx(spanCtx).Debug("sent item to processing goroutine")
				span.End()
			}
		}
	}
}

func consume[T any](ctx context.Context, c Consumer[T]) (item *Item[T], err error) {
	spanCtx, span := otel.Tracer("queue").Start(ctx, "consume")
	defer span.End()

	defer errRecover(&err)
	item, err = c.Consume(spanCtx)
	return
}

func processItems[T any](ctx context.Context, rt *Runtime, itemCh <-chan *itemWithId[T], p Processor[T]) func() error {
	return func() error {
		g, gctx := errgroup.WithContext(ctx)
		g.SetLimit(rt.maxConcurrentProcessors)

		tracer := otel.Tracer("queue")
		for {
			spanCtx, span := tracer.Start(ctx, "processItems", trace.WithAttributes(attribute.Int("queue.max_concurrent_processors", rt.maxConcurrentProcessors)))

			select {
			case <-gctx.Done():
				rt.log.Ctx(spanCtx).Warn("context cancelled")
				span.End()
				return g.Wait()
			case itemWithId := <-itemCh:
				if itemWithId == nil {
					rt.log.Ctx(spanCtx).Info("item channel was closed")
					return g.Wait()
				}
				id := itemWithId.id
				item := itemWithId.item
				span.SetAttributes(attribute.Int64("queue.item.index", id))
				g.Go(func() error {
					defer span.End()

					err := process[T](spanCtx, p, item.Value)
					if err != nil {
						rt.log.Ctx(spanCtx).Error("failed to process item", zap.Error(err))
					}
					return nil
				})
			}
		}
	}
}

func process[T any](ctx context.Context, p Processor[T], value T) (err error) {
	spanCtx, span := otel.Tracer("queue").Start(ctx, "process")
	defer span.End()

	defer errRecover(&err)
	err = p.Process(spanCtx, value)
	return
}

func sequential[T any](c Consumer[T], p Processor[T]) func(context.Context, *Runtime) error {
	return func(ctx context.Context, rt *Runtime) error {
		var itemIdx int64 = math.MinInt64
		tracer := otel.Tracer("queue")
		for {
			itemIdx += 1
			if itemIdx == math.MaxInt64 {
				itemIdx = math.MinInt64
			}
			spanCtx, span := tracer.Start(ctx, "sequential", trace.WithAttributes(attribute.Int64("queue.item.index", itemIdx)))
			select {
			case <-spanCtx.Done():
				rt.log.Ctx(spanCtx).Warn("context cancelled before item could be consumed")
				return nil
			default:
			}

			item, err := consume(spanCtx, c)
			if err == ErrEndOfItems {
				rt.log.Ctx(spanCtx).Info("end of queue")
				return nil
			}
			if err != nil {
				rt.log.Ctx(spanCtx).Error("failed to consume item from queue", zap.Error(err))
				span.End()
				continue
			}
			if item == nil {
				rt.log.Ctx(spanCtx).Info("received a nil item from the queue")
				span.End()
				continue
			}
			select {
			case <-spanCtx.Done():
				rt.log.Ctx(spanCtx).Warn("context cancelled before item could be processed")
				return nil
			default:
			}

			err = process[T](spanCtx, p, item.Value)
			if err != nil {
				rt.log.Ctx(spanCtx).Error("failed to process item from queue", zap.Error(err))
				span.End()
				continue
			}
			span.End()
		}
	}
}

func errRecover(err *error) {
	r := recover()
	if r == nil {
		return
	}
	rerr, ok := r.(error)
	if !ok {
		// TODO
		return
	}
	*err = rerr
}
