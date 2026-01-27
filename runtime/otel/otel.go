// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package otel

import (
	"context"
	"errors"

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

func BuildTraceIDRatioBasedSampler(ratio config.Reader[float64]) bedrock.Builder[sdktrace.Sampler] {
	return bedrock.BuilderFunc[sdktrace.Sampler](func(ctx context.Context) (sdktrace.Sampler, error) {
		sampler := sdktrace.TraceIDRatioBased(config.Must(ctx, ratio))

		return sampler, nil
	})
}

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
}

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
) bedrock.Builder[Runtime[T, M, L, R]] {
	return bedrock.BuilderFunc[Runtime[T, M, L, R]](func(ctx context.Context) (Runtime[T, M, L, R], error) {
		textMapPropagator := bedrock.MustBuild(ctx, textMapPropagatorBuilder)
		tracerProvider := bedrock.MustBuild(ctx, tracerProviderBuilder)
		meterProvider := bedrock.MustBuild(ctx, meterProviderBuilder)
		loggerProvider := bedrock.MustBuild(ctx, loggerProviderBuilder)
		runtime := bedrock.MustBuild(ctx, runtimeBuilder)

		return Runtime[T, M, L, R]{
			textMapPropagator: textMapPropagator,
			tracerProvider:    tracerProvider,
			meterProvider:     meterProvider,
			loggerProvider:    loggerProvider,
			runtime:           runtime,
		}, nil
	})
}

type shutdownInterface interface {
	Shutdown(ctx context.Context) error
}

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
