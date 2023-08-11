// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package queue

import (
	"context"

	"github.com/uptrace/opentelemetry-go-extra/otelzap"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

// Item
type Item[T any] struct {
	Value T
}

// Consumer
type Consumer[T any] interface {
	Consume(context.Context) (*Item[T], error)
}

// Processor
type Processor[T any] interface {
	Process(context.Context, T) error
}

// Option
type Option func(*Runtime)

func Logger(logger *zap.Logger) Option {
	return func(r *Runtime) {
		r.log = logger
	}
}

// MaxConcurrentProcessors
func MaxConcurrentProcessors(n int) Option {
	return func(r *Runtime) {
		r.maxConcurrentProcessors = n
	}
}

// Pipe registers a consumer/processor pair with the queue runtime.
func Pipe[T any](c Consumer[T], p Processor[T]) Option {
	return func(r *Runtime) {
		r.qps = append(r.qps, pipe(c, p))
	}
}

// Sequential
func Sequential[T any](c Consumer[T], p Processor[T]) Option {
	return func(r *Runtime) {
		r.qps = append(r.qps, sequential(c, p))
	}
}

// Runtime
type Runtime struct {
	log                     *zap.Logger
	qps                     []func(context.Context, *Runtime) error
	maxConcurrentProcessors int
}

func NewRuntime(opts ...Option) *Runtime {
	r := &Runtime{
		maxConcurrentProcessors: -1,
	}
	for _, opt := range opts {
		opt(r)
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

func pipe[T any](c Consumer[T], p Processor[T]) func(context.Context, *Runtime) error {
	return func(ctx context.Context, rt *Runtime) error {
		itemCh := make(chan *Item[T])
		g, gctx := errgroup.WithContext(ctx)
		g.Go(consumeQueue(gctx, rt, itemCh, c))
		g.Go(processItems(gctx, rt, itemCh, p))
		return g.Wait()
	}
}

func consumeQueue[T any](ctx context.Context, rt *Runtime, itemCh chan<- *Item[T], c Consumer[T]) func() error {
	return func() error {
		defer close(itemCh)
		log := otelzap.New(rt.log)

		for {
			spanCtx, span := otel.Tracer("queue").Start(ctx, "consumeQueue")
			item, err := consume[T](spanCtx, c)
			if err != nil {
				log.Ctx(spanCtx).Error("encountered error when consuming item from queue", zap.Error(err))
				span.End()
				continue
			}
			if item == nil {
				log.Ctx(spanCtx).Warn("queue returned a nil item")
				span.End()
				continue
			}
			select {
			case <-ctx.Done():
				log.Ctx(spanCtx).Warn("context was cancelled before item could be processed")
				span.End()
				return nil
			case itemCh <- item:
				log.Ctx(spanCtx).Debug("sent item to processing goroutine")
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

func processItems[T any](ctx context.Context, rt *Runtime, itemCh <-chan *Item[T], p Processor[T]) func() error {
	return func() error {
		log := otelzap.New(rt.log)

		g, gctx := errgroup.WithContext(ctx)
		g.SetLimit(rt.maxConcurrentProcessors)

		for {
			var item *Item[T]
			select {
			case <-gctx.Done():
				log.Warn("context cancelled")
				return g.Wait()
			case item = <-itemCh:
			}
			if item == nil {
				log.Info("item channel was closed")
				return g.Wait()
			}
			g.Go(func() error {
				err := process[T](ctx, p, item)
				if err != nil {
					// TODO
				}
				return nil
			})
		}
	}
}

func process[T any](ctx context.Context, p Processor[T], item *Item[T]) (err error) {
	spanCtx, span := otel.Tracer("queue").Start(ctx, "process")
	defer span.End()

	defer errRecover(&err)
	err = p.Process(spanCtx, item.Value)
	return
}

func sequential[T any](c Consumer[T], p Processor[T]) func(context.Context, *Runtime) error {
	return func(ctx context.Context, rt *Runtime) error {
		for {
			item, err := consume(ctx, c)
			if err != nil {
				continue
			}
			if item == nil {
				return nil
			}
			err = process[T](ctx, p, item)
			if err != nil {
				continue
			}
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
