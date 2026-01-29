// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package otel

import (
	"context"
	"errors"
	"time"

	"github.com/z5labs/bedrock"
	"github.com/z5labs/bedrock/config"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// BuildTraceIDRatioBasedSampler returns a Builder that creates a trace sampler based on
// the trace ID. The ratio parameter determines the fraction of traces to sample, where
// 0.0 samples no traces and 1.0 samples all traces.
func BuildTraceIDRatioBasedSampler(ratio config.Reader[float64]) bedrock.Builder[sdktrace.Sampler] {
	return bedrock.BuilderFunc[sdktrace.Sampler](func(ctx context.Context) (sdktrace.Sampler, error) {
		sampler := sdktrace.TraceIDRatioBased(config.Must(ctx, ratio))

		return sampler, nil
	})
}

// BuildBatchSpanProcessor returns a Builder that creates a span processor which batches
// spans before exporting them using the provided exporter. Batching improves efficiency
// by reducing the number of export calls.
func BuildBatchSpanProcessor[E sdktrace.SpanExporter](
	exporterBuilder bedrock.Builder[E],
	// TODO: add options
) bedrock.Builder[sdktrace.SpanProcessor] {
	return bedrock.BuilderFunc[sdktrace.SpanProcessor](func(ctx context.Context) (sdktrace.SpanProcessor, error) {
		bsp := sdktrace.NewBatchSpanProcessor(
			bedrock.MustBuild(ctx, exporterBuilder),
		)

		return bsp, nil
	})
}

// BuildTracerProvider returns a Builder that creates a TracerProvider configured with
// the provided resource, sampler, and span processor. The TracerProvider is the entry
// point for the tracing API and manages Tracer instances.
func BuildTracerProvider[S sdktrace.Sampler, P sdktrace.SpanProcessor](
	resourceBuilder bedrock.Builder[*resource.Resource],
	samplerBuilder bedrock.Builder[S],
	spanProcessorBuilder bedrock.Builder[P],
) bedrock.Builder[*sdktrace.TracerProvider] {
	return bedrock.BuilderFunc[*sdktrace.TracerProvider](func(ctx context.Context) (*sdktrace.TracerProvider, error) {
		tp := sdktrace.NewTracerProvider(
			sdktrace.WithResource(bedrock.MustBuild(ctx, resourceBuilder)),
			sdktrace.WithSampler(bedrock.MustBuild(ctx, samplerBuilder)),
			sdktrace.WithSpanProcessor(bedrock.MustBuild(ctx, spanProcessorBuilder)),
		)

		return tp, nil
	})
}

// BuildPeriodicReader returns a Builder that creates a metric reader which periodically
// exports metrics using the provided exporter. The reader collects and exports metrics
// at regular intervals.
func BuildPeriodicReader[E sdkmetric.Exporter](
	exporterBuilder bedrock.Builder[E],
	// TODO: add options
) bedrock.Builder[*sdkmetric.PeriodicReader] {
	return bedrock.BuilderFunc[*sdkmetric.PeriodicReader](func(ctx context.Context) (*sdkmetric.PeriodicReader, error) {
		pr := sdkmetric.NewPeriodicReader(
			bedrock.MustBuild(ctx, exporterBuilder),
		)

		return pr, nil
	})
}

// BuildMeterProvider returns a Builder that creates a MeterProvider configured with
// the provided resource and reader. The MeterProvider is the entry point for the
// metrics API and manages Meter instances.
func BuildMeterProvider[R sdkmetric.Reader](
	resourceBuilder bedrock.Builder[*resource.Resource],
	readerBuilder bedrock.Builder[R],
) bedrock.Builder[*sdkmetric.MeterProvider] {
	return bedrock.BuilderFunc[*sdkmetric.MeterProvider](func(ctx context.Context) (*sdkmetric.MeterProvider, error) {
		mp := sdkmetric.NewMeterProvider(
			sdkmetric.WithResource(bedrock.MustBuild(ctx, resourceBuilder)),
			sdkmetric.WithReader(bedrock.MustBuild(ctx, readerBuilder)),
		)

		return mp, nil
	})
}

// BuildBatchLogProcessor returns a Builder that creates a log processor which batches
// log records before exporting them using the provided exporter. Batching improves
// efficiency by reducing the number of export calls.
func BuildBatchLogProcessor[E sdklog.Exporter](
	exporterBuilder bedrock.Builder[E],
	// TODO: add options
) bedrock.Builder[*sdklog.BatchProcessor] {
	return bedrock.BuilderFunc[*sdklog.BatchProcessor](func(ctx context.Context) (*sdklog.BatchProcessor, error) {
		bp := sdklog.NewBatchProcessor(
			bedrock.MustBuild(ctx, exporterBuilder),
		)

		return bp, nil
	})
}

// BuildLoggerProvider returns a Builder that creates a LoggerProvider configured with
// the provided resource and processor. The LoggerProvider is the entry point for the
// logging API and manages Logger instances.
func BuildLoggerProvider[P sdklog.Processor](
	resourceBuilder bedrock.Builder[*resource.Resource],
	processorBuilder bedrock.Builder[P],
) bedrock.Builder[*sdklog.LoggerProvider] {
	return bedrock.BuilderFunc[*sdklog.LoggerProvider](func(ctx context.Context) (*sdklog.LoggerProvider, error) {
		lp := sdklog.NewLoggerProvider(
			sdklog.WithResource(bedrock.MustBuild(ctx, resourceBuilder)),
			sdklog.WithProcessor(bedrock.MustBuild(ctx, processorBuilder)),
		)

		return lp, nil
	})
}

// RuntimeOptions holds configuration options for the OpenTelemetry runtime wrapper
type RuntimeOptions struct {
	shutdownGracePeriod time.Duration
}

// RuntimeOption defines a function type for configuring RuntimeOptions
type RuntimeOption func(*RuntimeOptions)

// ShutdownGracePeriod sets the duration to wait for OpenTelemetry providers to shut down.
//
// Default is 30 seconds.
func ShutdownGracePeriod(d time.Duration) RuntimeOption {
	return func(o *RuntimeOptions) {
		o.shutdownGracePeriod = d
	}
}

// Runtime wraps a bedrock.Runtime with OpenTelemetry providers for tracing, metrics,
// and logging. When Run is called, it registers the providers globally and ensures
// they are properly shut down when the wrapped runtime completes.
type Runtime[
	T trace.TracerProvider,
	M metric.MeterProvider,
	L log.LoggerProvider,
	R bedrock.Runtime,
] struct {
	textMapPropagator propagation.TextMapPropagator
	tracerProvider    T
	meterProvider     M
	loggerProvider    L
	runtime           R

	shutdownGracePeriod time.Duration
}

// BuildRuntime returns a Builder that creates a Runtime wrapping the provided runtime
// with OpenTelemetry providers. The text map propagator is used for context propagation
// across service boundaries.
func BuildRuntime[
	T trace.TracerProvider,
	M metric.MeterProvider,
	L log.LoggerProvider,
	R bedrock.Runtime,
](
	textMapPropagatorBuilder bedrock.Builder[propagation.TextMapPropagator],
	tracerProviderBuilder bedrock.Builder[T],
	meterProviderBuilder bedrock.Builder[M],
	loggerProviderBuilder bedrock.Builder[L],
	runtimeBuilder bedrock.Builder[R],
	opts ...RuntimeOption,
) bedrock.Builder[Runtime[T, M, L, R]] {
	return bedrock.BuilderFunc[Runtime[T, M, L, R]](func(ctx context.Context) (Runtime[T, M, L, R], error) {
		ro := &RuntimeOptions{
			// 30 seconds aligns with the K8s default terminationGracePeriodSeconds
			shutdownGracePeriod: 30 * time.Second,
		}
		for _, opt := range opts {
			opt(ro)
		}

		textMapPropagator := bedrock.MustBuild(ctx, textMapPropagatorBuilder)
		tracerProvider := bedrock.MustBuild(ctx, tracerProviderBuilder)
		meterProvider := bedrock.MustBuild(ctx, meterProviderBuilder)
		loggerProvider := bedrock.MustBuild(ctx, loggerProviderBuilder)
		runtime := bedrock.MustBuild(ctx, runtimeBuilder)

		return Runtime[T, M, L, R]{
			textMapPropagator:   textMapPropagator,
			tracerProvider:      tracerProvider,
			meterProvider:       meterProvider,
			loggerProvider:      loggerProvider,
			runtime:             runtime,
			shutdownGracePeriod: ro.shutdownGracePeriod,
		}, nil
	})
}

type shutdownInterface interface {
	Shutdown(ctx context.Context) error
}

// Run registers the OpenTelemetry providers globally, executes the wrapped runtime,
// and shuts down all providers when complete. Provider shutdown errors are joined
// with any error from the wrapped runtime.
func (r Runtime[T, M, L, R]) Run(ctx context.Context) (err error) {
	shutdownFuncs := make([]func(context.Context) error, 3)

	otel.SetTextMapPropagator(r.textMapPropagator)

	otel.SetTracerProvider(r.tracerProvider)
	if sd, ok := any(r.tracerProvider).(shutdownInterface); ok {
		shutdownFuncs[0] = sd.Shutdown
	}

	otel.SetMeterProvider(r.meterProvider)
	if sd, ok := any(r.meterProvider).(shutdownInterface); ok {
		shutdownFuncs[1] = sd.Shutdown
	}

	global.SetLoggerProvider(r.loggerProvider)
	if sd, ok := any(r.loggerProvider).(shutdownInterface); ok {
		shutdownFuncs[2] = sd.Shutdown
	}

	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), r.shutdownGracePeriod)
		defer cancel()

		shutdownErrs := make([]error, 3)
		for i, shutdown := range shutdownFuncs {
			if shutdown == nil {
				continue
			}
			shutdownErrs[i] = shutdown(ctx)
		}
		err = errors.Join(err, errors.Join(shutdownErrs...))
	}()

	return r.runtime.Run(ctx)
}
