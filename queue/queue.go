// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package queue

import (
	"context"

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

// Runtime
type Runtime struct {
	qps                     []func(context.Context) error
	maxConcurrentProcessors int
}

func NewRuntime(opts ...Option) *Runtime {
	r := &Runtime{}
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
			return qp(gctx)
		})
	}
	return g.Wait()
}

func pipe[T any](c Consumer[T], p Processor[T]) func(context.Context) error {
	return func(ctx context.Context) error {
		itemCh := make(chan *Item[T])
		g, gctx := errgroup.WithContext(ctx)
		g.Go(consumeQueue(gctx, itemCh, c))
		g.Go(processItems(gctx, itemCh, p))
		return g.Wait()
	}
}

func consumeQueue[T any](ctx context.Context, itemCh chan<- *Item[T], c Consumer[T]) func() error {
	return func() error {
		defer close(itemCh)
		for {
			item, err := consume[T](ctx, c)
			if err != nil {
				continue
			}
			if item == nil {
				continue
			}
			select {
			case <-ctx.Done():
				return nil
			case itemCh <- item:
			}
		}
	}
}

func consume[T any](ctx context.Context, c Consumer[T]) (item *Item[T], err error) {
	defer errRecover(&err)
	item, err = c.Consume(ctx)
	return
}

func processItems[T any](ctx context.Context, itemCh <-chan *Item[T], p Processor[T]) func() error {
	return func() error {
		g, gctx := errgroup.WithContext(ctx)
		for {
			var item *Item[T]
			select {
			case <-gctx.Done():
				return g.Wait()
			case item = <-itemCh:
			}
			if item == nil {
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
	defer errRecover(&err)
	err = p.Process(ctx, item.Value)
	return
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
