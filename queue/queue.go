// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package queue

import (
	"context"
	"log/slog"

	"github.com/z5labs/app/pkg/otelconfig"
	"github.com/z5labs/app/pkg/otelslog"
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

type runtimeOptions struct {
	logHandler slog.Handler
	otelIniter otelconfig.Initializer
	qps        []func(context.Context, *Runtime) error
}

type RuntimeOption func(*runtimeOptions)

// LogHandler
func LogHandler(h slog.Handler) RuntimeOption {
	return func(ro *runtimeOptions) {
		ro.logHandler = otelslog.NewHandler(h)
	}
}

// InitTracerProvider
func InitTracerProvider(initer otelconfig.Initializer) RuntimeOption {
	return func(ro *runtimeOptions) {
		ro.otelIniter = initer
	}
}

// Sequential
func Sequential[T any](c Consumer[T], p Processor[T]) RuntimeOption {
	return func(ro *runtimeOptions) {
		ro.qps = append(ro.qps, sequential(c, p))
	}
}

type pipeOptions struct {
	maxConcurrentProcessors uint
	propagator              propagation.TextMapPropagator
}

// PipeOption
type PipeOption func(*pipeOptions)

// MaxConcurrentProcessors
func MaxConcurrentProcessors(n uint) PipeOption {
	return func(po *pipeOptions) {
		po.maxConcurrentProcessors = n
	}
}

// Pipe
func Pipe[T any](c Consumer[T], p Processor[T], opts ...PipeOption) RuntimeOption {
	return func(ro *runtimeOptions) {
		po := &pipeOptions{
			propagator: propagation.Baggage{},
		}
		for _, opt := range opts {
			opt(po)
		}
		ro.qps = append(ro.qps, pipe(c, p, po))
	}
}

// Runtime
type Runtime struct {
	log        *slog.Logger
	otelIniter otelconfig.Initializer
	qps        []func(context.Context, *Runtime) error
}

// NewRuntime
func NewRuntime(opts ...RuntimeOption) *Runtime {
	ro := &runtimeOptions{
		logHandler: noopLogHandler{},
		otelIniter: otelconfig.Noop,
	}
	for _, opt := range opts {
		opt(ro)
	}
	return &Runtime{
		log:        slog.New(ro.logHandler),
		otelIniter: ro.otelIniter,
		qps:        ro.qps,
	}
}

// Run implements the app.Runtime interface.
func (rt *Runtime) Run(ctx context.Context) error {
	tp, err := rt.otelIniter.Init()
	if err != nil {
		rt.log.ErrorContext(ctx, "failed to initialize otel", slogfield.Error(err))
		return err
	}
	otel.SetTracerProvider(tp)

	g, gctx := errgroup.WithContext(ctx)
	for _, qp := range rt.qps {
		qp := qp
		g.Go(func() error {
			return qp(gctx, rt)
		})
	}
	return g.Wait()
}

type item[T any] struct {
	value T

	// for concurrent Consumer-Processor implemetations
	// the otel context needs to be propagated between goroutines
	carrier propagation.TextMapCarrier
}

func sequential[T any](c Consumer[T], p Processor[T]) func(context.Context, *Runtime) error {
	return func(ctx context.Context, rt *Runtime) error {
		tracer := otel.Tracer("queue")
		for {
			select {
			case <-ctx.Done():
				return nil
			default:
			}

			spanCtx, span := tracer.Start(ctx, "sequential")
			item, err := consume(spanCtx, c)
			if err != nil {
				rt.log.ErrorContext(spanCtx, "failed to consume", slogfield.Error(err))
				span.End()
				continue
			}

			select {
			case <-ctx.Done():
				return nil
			default:
			}

			err = process(spanCtx, p, item.value)
			if err != nil {
				rt.log.ErrorContext(spanCtx, "failed to process", slogfield.Error(err))
			}
			span.End()
		}
	}
}

func pipe[T any](c Consumer[T], p Processor[T], po *pipeOptions) func(context.Context, *Runtime) error {
	return func(ctx context.Context, rt *Runtime) error {
		itemCh := make(chan *item[T])

		g, gctx := errgroup.WithContext(ctx)
		g.Go(consumeItems(gctx, rt, c, itemCh, po))
		g.Go(processItems(gctx, rt, p, itemCh, po))
		return g.Wait()
	}
}

func consumeItems[T any](ctx context.Context, rt *Runtime, c Consumer[T], itemCh chan<- *item[T], po *pipeOptions) func() error {
	return func() error {
		defer close(itemCh)

		tracer := otel.Tracer("queue")
		for {
			spanCtx, span := tracer.Start(ctx, "consumeItems")

			select {
			case <-spanCtx.Done():
				span.End()
				return nil
			default:
			}

			item, err := consume(spanCtx, c)
			if err != nil {
				rt.log.ErrorContext(spanCtx, "failed to consume", slogfield.Error(err))
				span.End()
				continue
			}

			item.carrier = make(propagation.MapCarrier)
			po.propagator.Inject(spanCtx, item.carrier)

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

func processItems[T any](ctx context.Context, rt *Runtime, p Processor[T], itemCh <-chan *item[T], po *pipeOptions) func() error {
	return func() error {
		maxProcs := -1
		if po.maxConcurrentProcessors > 0 {
			maxProcs = int(po.maxConcurrentProcessors)
		}

		g, gctx := errgroup.WithContext(ctx)
		g.SetLimit(maxProcs)

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

			propCtx := po.propagator.Extract(gctx, i.carrier)
			g.Go(processItem(propCtx, p, i))
		}
	}
}

func processItem[T any](ctx context.Context, p Processor[T], i *item[T]) func() error {
	return func() error {
		spanCtx, span := otel.Tracer("queue").Start(ctx, "processItem")
		defer span.End()

		return process(spanCtx, p, i.value)
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
		return
	}
	*err = rerr
}

type noopLogHandler struct{}

func (noopLogHandler) Enabled(_ context.Context, _ slog.Level) bool  { return true }
func (noopLogHandler) Handle(_ context.Context, _ slog.Record) error { return nil }
func (h noopLogHandler) WithAttrs(_ []slog.Attr) slog.Handler        { return h }
func (h noopLogHandler) WithGroup(name string) slog.Handler          { return h }
