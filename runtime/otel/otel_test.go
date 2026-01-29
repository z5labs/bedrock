// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package otel

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/z5labs/bedrock"
	"github.com/z5labs/bedrock/config"
	"github.com/z5labs/bedrock/runtime/otel/noop"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// mockErrorHandler implements otel.ErrorHandler for testing
type mockErrorHandler struct{}

func (m mockErrorHandler) Handle(err error) {}

// mockTracerProvider tracks shutdown calls for testing
type mockTracerProvider struct {
	trace.TracerProvider
	shutdownCalled atomic.Bool
	shutdownErr    error
}

func (m *mockTracerProvider) Shutdown(ctx context.Context) error {
	m.shutdownCalled.Store(true)
	return m.shutdownErr
}

// mockMeterProvider tracks shutdown calls for testing
type mockMeterProvider struct {
	metric.MeterProvider
	shutdownCalled atomic.Bool
	shutdownErr    error
}

func (m *mockMeterProvider) Shutdown(ctx context.Context) error {
	m.shutdownCalled.Store(true)
	return m.shutdownErr
}

// mockLoggerProvider tracks shutdown calls for testing
type mockLoggerProvider struct {
	log.LoggerProvider
	shutdownCalled atomic.Bool
	shutdownErr    error
}

func (m *mockLoggerProvider) Shutdown(ctx context.Context) error {
	m.shutdownCalled.Store(true)
	return m.shutdownErr
}

// noShutdownTracerProvider is a TracerProvider without Shutdown method
type noShutdownTracerProvider struct {
	trace.TracerProvider
}

// noShutdownMeterProvider is a MeterProvider without Shutdown method
type noShutdownMeterProvider struct {
	metric.MeterProvider
}

// noShutdownLoggerProvider is a LoggerProvider without Shutdown method
type noShutdownLoggerProvider struct {
	log.LoggerProvider
}

// buildTestErrorHandler creates an ErrorHandler builder for testing
func buildTestErrorHandler() bedrock.Builder[otel.ErrorHandler] {
	return bedrock.BuilderOf[otel.ErrorHandler](mockErrorHandler{})
}

// buildTestResource creates a resource builder for testing
func buildTestResource() bedrock.Builder[*resource.Resource] {
	return bedrock.MemoizeBuilder(bedrock.BuilderFunc[*resource.Resource](func(ctx context.Context) (*resource.Resource, error) {
		return resource.New(ctx)
	}))
}

// buildTestTracerProvider creates a TracerProvider builder with noop exporters for testing
func buildTestTracerProvider(resourceB bedrock.Builder[*resource.Resource]) bedrock.Builder[*sdktrace.TracerProvider] {
	return BuildTracerProvider(
		resourceB,
		BuildTraceIDRatioBasedSampler(config.ReaderOf(1.0)),
		BuildBatchSpanProcessor(noop.BuildSpanExporter()),
	)
}

// buildTestMeterProvider creates a MeterProvider builder with noop exporters for testing
func buildTestMeterProvider(resourceB bedrock.Builder[*resource.Resource]) bedrock.Builder[*sdkmetric.MeterProvider] {
	return BuildMeterProvider(
		resourceB,
		BuildPeriodicReader(noop.BuildMetricExporter()),
	)
}

// buildTestLoggerProvider creates a LoggerProvider builder with noop exporters for testing
func buildTestLoggerProvider(resourceB bedrock.Builder[*resource.Resource]) bedrock.Builder[*sdklog.LoggerProvider] {
	return BuildLoggerProvider(
		resourceB,
		BuildBatchLogProcessor(noop.BuildLogExporter()),
	)
}

func TestRuntime_Run(t *testing.T) {
	t.Run("success with noop exporters", func(t *testing.T) {
		resourceB := buildTestResource()

		runtimeB := BuildRuntime(
			buildTestErrorHandler(),
			bedrock.BuilderOf(propagation.NewCompositeTextMapPropagator(
				propagation.Baggage{},
				propagation.TraceContext{},
			)),
			buildTestTracerProvider(resourceB),
			buildTestMeterProvider(resourceB),
			buildTestLoggerProvider(resourceB),
			bedrock.BuilderOf(bedrock.RuntimeFunc(func(ctx context.Context) error {
				return nil
			})),
		)

		rt, err := runtimeB.Build(context.Background())
		require.NoError(t, err)

		err = rt.Run(context.Background())
		require.NoError(t, err)
	})

	t.Run("wrapped runtime is called", func(t *testing.T) {
		resourceB := buildTestResource()
		runtimeCalled := false

		runtimeB := BuildRuntime(
			buildTestErrorHandler(),
			bedrock.BuilderOf(propagation.NewCompositeTextMapPropagator()),
			buildTestTracerProvider(resourceB),
			buildTestMeterProvider(resourceB),
			buildTestLoggerProvider(resourceB),
			bedrock.BuilderOf(bedrock.RuntimeFunc(func(ctx context.Context) error {
				runtimeCalled = true
				return nil
			})),
		)

		rt, err := runtimeB.Build(context.Background())
		require.NoError(t, err)

		err = rt.Run(context.Background())
		require.NoError(t, err)
		require.True(t, runtimeCalled, "wrapped runtime should be called")
	})

	t.Run("propagates runtime error", func(t *testing.T) {
		resourceB := buildTestResource()
		expectedErr := errors.New("runtime failed")

		runtimeB := BuildRuntime(
			buildTestErrorHandler(),
			bedrock.BuilderOf(propagation.NewCompositeTextMapPropagator()),
			buildTestTracerProvider(resourceB),
			buildTestMeterProvider(resourceB),
			buildTestLoggerProvider(resourceB),
			bedrock.BuilderOf(bedrock.RuntimeFunc(func(ctx context.Context) error {
				return expectedErr
			})),
		)

		rt, err := runtimeB.Build(context.Background())
		require.NoError(t, err)

		err = rt.Run(context.Background())
		require.ErrorIs(t, err, expectedErr)
	})

	t.Run("context cancellation propagated to inner runtime", func(t *testing.T) {
		resourceB := buildTestResource()
		contextCancelled := false

		runtimeB := BuildRuntime(
			buildTestErrorHandler(),
			bedrock.BuilderOf(propagation.NewCompositeTextMapPropagator()),
			buildTestTracerProvider(resourceB),
			buildTestMeterProvider(resourceB),
			buildTestLoggerProvider(resourceB),
			bedrock.BuilderOf(bedrock.RuntimeFunc(func(ctx context.Context) error {
				<-ctx.Done()
				contextCancelled = true
				return ctx.Err()
			})),
		)

		rt, err := runtimeB.Build(context.Background())
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		err = rt.Run(ctx)
		require.ErrorIs(t, err, context.Canceled)
		require.True(t, contextCancelled, "context cancellation should propagate to inner runtime")
	})
}

func TestRuntime_Run_Shutdown(t *testing.T) {
	t.Run("shutdown called on success", func(t *testing.T) {
		mockTracer := &mockTracerProvider{}
		mockMeter := &mockMeterProvider{}
		mockLogger := &mockLoggerProvider{}

		rt := Runtime[mockErrorHandler, *mockTracerProvider, *mockMeterProvider, *mockLoggerProvider, bedrock.Runtime]{
			errorHandler:      mockErrorHandler{},
			textMapPropagator: propagation.NewCompositeTextMapPropagator(),
			tracerProvider:    mockTracer,
			meterProvider:     mockMeter,
			loggerProvider:    mockLogger,
			runtime: bedrock.RuntimeFunc(func(ctx context.Context) error {
				return nil
			}),
		}

		err := rt.Run(context.Background())
		require.NoError(t, err)
		require.True(t, mockTracer.shutdownCalled.Load(), "tracer provider shutdown should be called")
		require.True(t, mockMeter.shutdownCalled.Load(), "meter provider shutdown should be called")
		require.True(t, mockLogger.shutdownCalled.Load(), "logger provider shutdown should be called")
	})

	t.Run("shutdown called on runtime error", func(t *testing.T) {
		mockTracer := &mockTracerProvider{}
		mockMeter := &mockMeterProvider{}
		mockLogger := &mockLoggerProvider{}
		runtimeErr := errors.New("runtime failed")

		rt := Runtime[mockErrorHandler, *mockTracerProvider, *mockMeterProvider, *mockLoggerProvider, bedrock.Runtime]{
			errorHandler:      mockErrorHandler{},
			textMapPropagator: propagation.NewCompositeTextMapPropagator(),
			tracerProvider:    mockTracer,
			meterProvider:     mockMeter,
			loggerProvider:    mockLogger,
			runtime: bedrock.RuntimeFunc(func(ctx context.Context) error {
				return runtimeErr
			}),
		}

		err := rt.Run(context.Background())
		require.ErrorIs(t, err, runtimeErr)
		require.True(t, mockTracer.shutdownCalled.Load(), "tracer provider shutdown should be called even on error")
		require.True(t, mockMeter.shutdownCalled.Load(), "meter provider shutdown should be called even on error")
		require.True(t, mockLogger.shutdownCalled.Load(), "logger provider shutdown should be called even on error")
	})

	t.Run("joins shutdown errors", func(t *testing.T) {
		tracerShutdownErr := errors.New("tracer shutdown failed")
		meterShutdownErr := errors.New("meter shutdown failed")
		loggerShutdownErr := errors.New("logger shutdown failed")

		mockTracer := &mockTracerProvider{shutdownErr: tracerShutdownErr}
		mockMeter := &mockMeterProvider{shutdownErr: meterShutdownErr}
		mockLogger := &mockLoggerProvider{shutdownErr: loggerShutdownErr}

		rt := Runtime[mockErrorHandler, *mockTracerProvider, *mockMeterProvider, *mockLoggerProvider, bedrock.Runtime]{
			errorHandler:      mockErrorHandler{},
			textMapPropagator: propagation.NewCompositeTextMapPropagator(),
			tracerProvider:    mockTracer,
			meterProvider:     mockMeter,
			loggerProvider:    mockLogger,
			runtime: bedrock.RuntimeFunc(func(ctx context.Context) error {
				return nil
			}),
		}

		err := rt.Run(context.Background())
		require.Error(t, err)
		require.ErrorIs(t, err, tracerShutdownErr)
		require.ErrorIs(t, err, meterShutdownErr)
		require.ErrorIs(t, err, loggerShutdownErr)
	})

	t.Run("joins shutdown errors with runtime error", func(t *testing.T) {
		runtimeErr := errors.New("runtime failed")
		shutdownErr := errors.New("shutdown failed")

		mockTracer := &mockTracerProvider{shutdownErr: shutdownErr}
		mockMeter := &mockMeterProvider{}
		mockLogger := &mockLoggerProvider{}

		rt := Runtime[mockErrorHandler, *mockTracerProvider, *mockMeterProvider, *mockLoggerProvider, bedrock.Runtime]{
			errorHandler:      mockErrorHandler{},
			textMapPropagator: propagation.NewCompositeTextMapPropagator(),
			tracerProvider:    mockTracer,
			meterProvider:     mockMeter,
			loggerProvider:    mockLogger,
			runtime: bedrock.RuntimeFunc(func(ctx context.Context) error {
				return runtimeErr
			}),
		}

		err := rt.Run(context.Background())
		require.Error(t, err)
		require.ErrorIs(t, err, runtimeErr)
		require.ErrorIs(t, err, shutdownErr)
	})

	t.Run("handles non-shutdown providers", func(t *testing.T) {
		noShutdownTracer := &noShutdownTracerProvider{}
		noShutdownMeter := &noShutdownMeterProvider{}
		noShutdownLogger := &noShutdownLoggerProvider{}

		rt := Runtime[mockErrorHandler, *noShutdownTracerProvider, *noShutdownMeterProvider, *noShutdownLoggerProvider, bedrock.Runtime]{
			errorHandler:      mockErrorHandler{},
			textMapPropagator: propagation.NewCompositeTextMapPropagator(),
			tracerProvider:    noShutdownTracer,
			meterProvider:     noShutdownMeter,
			loggerProvider:    noShutdownLogger,
			runtime: bedrock.RuntimeFunc(func(ctx context.Context) error {
				return nil
			}),
		}

		err := rt.Run(context.Background())
		require.NoError(t, err)
	})
}

func TestBuildTraceIDRatioBasedSampler(t *testing.T) {
	t.Run("ratio 0.0", func(t *testing.T) {
		builder := BuildTraceIDRatioBasedSampler(config.ReaderOf(0.0))
		sampler, err := builder.Build(context.Background())
		require.NoError(t, err)
		require.NotNil(t, sampler)
	})

	t.Run("ratio 0.5", func(t *testing.T) {
		builder := BuildTraceIDRatioBasedSampler(config.ReaderOf(0.5))
		sampler, err := builder.Build(context.Background())
		require.NoError(t, err)
		require.NotNil(t, sampler)
	})

	t.Run("ratio 1.0", func(t *testing.T) {
		builder := BuildTraceIDRatioBasedSampler(config.ReaderOf(1.0))
		sampler, err := builder.Build(context.Background())
		require.NoError(t, err)
		require.NotNil(t, sampler)
	})

	t.Run("reader error panics", func(t *testing.T) {
		expectedErr := errors.New("reader failed")
		failingReader := config.ReaderFunc[float64](func(ctx context.Context) (config.Value[float64], error) {
			return config.Value[float64]{}, expectedErr
		})

		builder := BuildTraceIDRatioBasedSampler(failingReader)

		require.Panics(t, func() {
			_, _ = builder.Build(context.Background())
		})
	})

	t.Run("empty reader panics", func(t *testing.T) {
		builder := BuildTraceIDRatioBasedSampler(config.EmptyReader[float64]())

		require.Panics(t, func() {
			_, _ = builder.Build(context.Background())
		})
	})
}

func TestBuildBatchSpanProcessor(t *testing.T) {
	t.Run("valid exporter", func(t *testing.T) {
		builder := BuildBatchSpanProcessor(noop.BuildSpanExporter())
		processor, err := builder.Build(context.Background())
		require.NoError(t, err)
		require.NotNil(t, processor)
	})

	t.Run("exporter error panics", func(t *testing.T) {
		expectedErr := errors.New("exporter build failed")
		failingBuilder := bedrock.BuilderFunc[noop.SpanExporter](func(ctx context.Context) (noop.SpanExporter, error) {
			return noop.SpanExporter{}, expectedErr
		})

		builder := BuildBatchSpanProcessor(failingBuilder)

		require.Panics(t, func() {
			_, _ = builder.Build(context.Background())
		})
	})
}

func TestBuildTracerProvider(t *testing.T) {
	t.Run("valid components", func(t *testing.T) {
		resourceB := buildTestResource()
		samplerB := BuildTraceIDRatioBasedSampler(config.ReaderOf(1.0))
		processorB := BuildBatchSpanProcessor(noop.BuildSpanExporter())

		builder := BuildTracerProvider(resourceB, samplerB, processorB)
		provider, err := builder.Build(context.Background())
		require.NoError(t, err)
		require.NotNil(t, provider)
	})

	t.Run("resource error panics", func(t *testing.T) {
		expectedErr := errors.New("resource build failed")
		failingResourceB := bedrock.BuilderFunc[*resource.Resource](func(ctx context.Context) (*resource.Resource, error) {
			return nil, expectedErr
		})
		samplerB := BuildTraceIDRatioBasedSampler(config.ReaderOf(1.0))
		processorB := BuildBatchSpanProcessor(noop.BuildSpanExporter())

		builder := BuildTracerProvider(failingResourceB, samplerB, processorB)

		require.Panics(t, func() {
			_, _ = builder.Build(context.Background())
		})
	})

	t.Run("sampler error panics", func(t *testing.T) {
		resourceB := buildTestResource()
		failingReader := config.ReaderFunc[float64](func(ctx context.Context) (config.Value[float64], error) {
			return config.Value[float64]{}, errors.New("sampler read failed")
		})
		samplerB := BuildTraceIDRatioBasedSampler(failingReader)
		processorB := BuildBatchSpanProcessor(noop.BuildSpanExporter())

		builder := BuildTracerProvider(resourceB, samplerB, processorB)

		require.Panics(t, func() {
			_, _ = builder.Build(context.Background())
		})
	})

	t.Run("processor error panics", func(t *testing.T) {
		resourceB := buildTestResource()
		samplerB := BuildTraceIDRatioBasedSampler(config.ReaderOf(1.0))
		failingProcessorB := bedrock.BuilderFunc[sdktrace.SpanProcessor](func(ctx context.Context) (sdktrace.SpanProcessor, error) {
			return nil, errors.New("processor build failed")
		})

		builder := BuildTracerProvider(resourceB, samplerB, failingProcessorB)

		require.Panics(t, func() {
			_, _ = builder.Build(context.Background())
		})
	})
}

func TestBuildPeriodicReader(t *testing.T) {
	t.Run("valid exporter", func(t *testing.T) {
		builder := BuildPeriodicReader(noop.BuildMetricExporter())
		reader, err := builder.Build(context.Background())
		require.NoError(t, err)
		require.NotNil(t, reader)
	})

	t.Run("exporter error panics", func(t *testing.T) {
		expectedErr := errors.New("exporter build failed")
		failingBuilder := bedrock.BuilderFunc[noop.MetricExporter](func(ctx context.Context) (noop.MetricExporter, error) {
			return noop.MetricExporter{}, expectedErr
		})

		builder := BuildPeriodicReader(failingBuilder)

		require.Panics(t, func() {
			_, _ = builder.Build(context.Background())
		})
	})
}

func TestBuildMeterProvider(t *testing.T) {
	t.Run("valid components", func(t *testing.T) {
		resourceB := buildTestResource()
		readerB := BuildPeriodicReader(noop.BuildMetricExporter())

		builder := BuildMeterProvider(resourceB, readerB)
		provider, err := builder.Build(context.Background())
		require.NoError(t, err)
		require.NotNil(t, provider)
	})

	t.Run("resource error panics", func(t *testing.T) {
		expectedErr := errors.New("resource build failed")
		failingResourceB := bedrock.BuilderFunc[*resource.Resource](func(ctx context.Context) (*resource.Resource, error) {
			return nil, expectedErr
		})
		readerB := BuildPeriodicReader(noop.BuildMetricExporter())

		builder := BuildMeterProvider(failingResourceB, readerB)

		require.Panics(t, func() {
			_, _ = builder.Build(context.Background())
		})
	})

	t.Run("reader error panics", func(t *testing.T) {
		resourceB := buildTestResource()
		failingReaderB := bedrock.BuilderFunc[*sdkmetric.PeriodicReader](func(ctx context.Context) (*sdkmetric.PeriodicReader, error) {
			return nil, errors.New("reader build failed")
		})

		builder := BuildMeterProvider(resourceB, failingReaderB)

		require.Panics(t, func() {
			_, _ = builder.Build(context.Background())
		})
	})
}

func TestBuildBatchLogProcessor(t *testing.T) {
	t.Run("valid exporter", func(t *testing.T) {
		builder := BuildBatchLogProcessor(noop.BuildLogExporter())
		processor, err := builder.Build(context.Background())
		require.NoError(t, err)
		require.NotNil(t, processor)
	})

	t.Run("exporter error panics", func(t *testing.T) {
		expectedErr := errors.New("exporter build failed")
		failingBuilder := bedrock.BuilderFunc[noop.LogExporter](func(ctx context.Context) (noop.LogExporter, error) {
			return noop.LogExporter{}, expectedErr
		})

		builder := BuildBatchLogProcessor(failingBuilder)

		require.Panics(t, func() {
			_, _ = builder.Build(context.Background())
		})
	})
}

func TestBuildLoggerProvider(t *testing.T) {
	t.Run("valid components", func(t *testing.T) {
		resourceB := buildTestResource()
		processorB := BuildBatchLogProcessor(noop.BuildLogExporter())

		builder := BuildLoggerProvider(resourceB, processorB)
		provider, err := builder.Build(context.Background())
		require.NoError(t, err)
		require.NotNil(t, provider)
	})

	t.Run("resource error panics", func(t *testing.T) {
		expectedErr := errors.New("resource build failed")
		failingResourceB := bedrock.BuilderFunc[*resource.Resource](func(ctx context.Context) (*resource.Resource, error) {
			return nil, expectedErr
		})
		processorB := BuildBatchLogProcessor(noop.BuildLogExporter())

		builder := BuildLoggerProvider(failingResourceB, processorB)

		require.Panics(t, func() {
			_, _ = builder.Build(context.Background())
		})
	})

	t.Run("processor error panics", func(t *testing.T) {
		resourceB := buildTestResource()
		failingProcessorB := bedrock.BuilderFunc[*sdklog.BatchProcessor](func(ctx context.Context) (*sdklog.BatchProcessor, error) {
			return nil, errors.New("processor build failed")
		})

		builder := BuildLoggerProvider(resourceB, failingProcessorB)

		require.Panics(t, func() {
			_, _ = builder.Build(context.Background())
		})
	})
}

func TestBuildRuntime(t *testing.T) {
	t.Run("valid components", func(t *testing.T) {
		resourceB := buildTestResource()

		builder := BuildRuntime(
			buildTestErrorHandler(),
			bedrock.BuilderOf(propagation.NewCompositeTextMapPropagator()),
			buildTestTracerProvider(resourceB),
			buildTestMeterProvider(resourceB),
			buildTestLoggerProvider(resourceB),
			bedrock.BuilderOf(bedrock.RuntimeFunc(func(ctx context.Context) error {
				return nil
			})),
		)

		rt, err := builder.Build(context.Background())
		require.NoError(t, err)

		// Verify the runtime can be run successfully
		err = rt.Run(context.Background())
		require.NoError(t, err)
	})

	t.Run("error handler error panics", func(t *testing.T) {
		resourceB := buildTestResource()
		failingErrorHandlerB := bedrock.BuilderFunc[otel.ErrorHandler](func(ctx context.Context) (otel.ErrorHandler, error) {
			return nil, errors.New("error handler build failed")
		})

		builder := BuildRuntime(
			failingErrorHandlerB,
			bedrock.BuilderOf(propagation.NewCompositeTextMapPropagator()),
			buildTestTracerProvider(resourceB),
			buildTestMeterProvider(resourceB),
			buildTestLoggerProvider(resourceB),
			bedrock.BuilderOf(bedrock.RuntimeFunc(func(ctx context.Context) error {
				return nil
			})),
		)

		require.Panics(t, func() {
			_, _ = builder.Build(context.Background())
		})
	})

	t.Run("propagator error panics", func(t *testing.T) {
		resourceB := buildTestResource()
		failingPropagatorB := bedrock.BuilderFunc[propagation.TextMapPropagator](func(ctx context.Context) (propagation.TextMapPropagator, error) {
			return nil, errors.New("propagator build failed")
		})

		builder := BuildRuntime(
			buildTestErrorHandler(),
			failingPropagatorB,
			buildTestTracerProvider(resourceB),
			buildTestMeterProvider(resourceB),
			buildTestLoggerProvider(resourceB),
			bedrock.BuilderOf(bedrock.RuntimeFunc(func(ctx context.Context) error {
				return nil
			})),
		)

		require.Panics(t, func() {
			_, _ = builder.Build(context.Background())
		})
	})

	t.Run("tracer provider error panics", func(t *testing.T) {
		resourceB := buildTestResource()
		failingTracerB := bedrock.BuilderFunc[*sdktrace.TracerProvider](func(ctx context.Context) (*sdktrace.TracerProvider, error) {
			return nil, errors.New("tracer provider build failed")
		})

		builder := BuildRuntime(
			buildTestErrorHandler(),
			bedrock.BuilderOf(propagation.NewCompositeTextMapPropagator()),
			failingTracerB,
			buildTestMeterProvider(resourceB),
			buildTestLoggerProvider(resourceB),
			bedrock.BuilderOf(bedrock.RuntimeFunc(func(ctx context.Context) error {
				return nil
			})),
		)

		require.Panics(t, func() {
			_, _ = builder.Build(context.Background())
		})
	})

	t.Run("meter provider error panics", func(t *testing.T) {
		resourceB := buildTestResource()
		failingMeterB := bedrock.BuilderFunc[*sdkmetric.MeterProvider](func(ctx context.Context) (*sdkmetric.MeterProvider, error) {
			return nil, errors.New("meter provider build failed")
		})

		builder := BuildRuntime(
			buildTestErrorHandler(),
			bedrock.BuilderOf(propagation.NewCompositeTextMapPropagator()),
			buildTestTracerProvider(resourceB),
			failingMeterB,
			buildTestLoggerProvider(resourceB),
			bedrock.BuilderOf(bedrock.RuntimeFunc(func(ctx context.Context) error {
				return nil
			})),
		)

		require.Panics(t, func() {
			_, _ = builder.Build(context.Background())
		})
	})

	t.Run("logger provider error panics", func(t *testing.T) {
		resourceB := buildTestResource()
		failingLoggerB := bedrock.BuilderFunc[*sdklog.LoggerProvider](func(ctx context.Context) (*sdklog.LoggerProvider, error) {
			return nil, errors.New("logger provider build failed")
		})

		builder := BuildRuntime(
			buildTestErrorHandler(),
			bedrock.BuilderOf(propagation.NewCompositeTextMapPropagator()),
			buildTestTracerProvider(resourceB),
			buildTestMeterProvider(resourceB),
			failingLoggerB,
			bedrock.BuilderOf(bedrock.RuntimeFunc(func(ctx context.Context) error {
				return nil
			})),
		)

		require.Panics(t, func() {
			_, _ = builder.Build(context.Background())
		})
	})

	t.Run("runtime error panics", func(t *testing.T) {
		resourceB := buildTestResource()
		failingRuntimeB := bedrock.BuilderFunc[bedrock.Runtime](func(ctx context.Context) (bedrock.Runtime, error) {
			return nil, errors.New("runtime build failed")
		})

		builder := BuildRuntime(
			buildTestErrorHandler(),
			bedrock.BuilderOf(propagation.NewCompositeTextMapPropagator()),
			buildTestTracerProvider(resourceB),
			buildTestMeterProvider(resourceB),
			buildTestLoggerProvider(resourceB),
			failingRuntimeB,
		)

		require.Panics(t, func() {
			_, _ = builder.Build(context.Background())
		})
	})
}
