// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package otel provides OpenTelemetry integration for bedrock applications.
//
// This package wraps bedrock runtimes with OpenTelemetry tracing, metrics, and logging
// providers. It handles global provider registration and ensures proper shutdown of
// all providers when the runtime completes.
//
// # Core Components
//
// The package provides builders for the three OpenTelemetry signals:
//
//   - Tracing: BuildTracerProvider, BuildBatchSpanProcessor, BuildTraceIDRatioBasedSampler
//   - Metrics: BuildMeterProvider, BuildPeriodicReader
//   - Logging: BuildLoggerProvider, BuildBatchLogProcessor
//
// # Basic Usage
//
// Wrap an existing runtime with OpenTelemetry providers using BuildRuntime:
//
//	resourceB := bedrock.MemoizeBuilder(bedrock.BuilderFunc[*resource.Resource](func(ctx context.Context) (*resource.Resource, error) {
//	    return resource.New(ctx, resource.WithAttributes(...))
//	}))
//
//	tracerProviderB := otel.BuildTracerProvider(
//	    resourceB,
//	    otel.BuildTraceIDRatioBasedSampler(config.ReaderOf(1.0)),
//	    otel.BuildBatchSpanProcessor(exporterBuilder),
//	)
//
//	meterProviderB := otel.BuildMeterProvider(
//	    resourceB,
//	    otel.BuildPeriodicReader(metricExporterBuilder),
//	)
//
//	loggerProviderB := otel.BuildLoggerProvider(
//	    resourceB,
//	    otel.BuildBatchLogProcessor(logExporterBuilder),
//	)
//
//	runtimeB := otel.BuildRuntime(
//	    bedrock.BuilderOf(propagation.NewCompositeTextMapPropagator(
//	        propagation.Baggage{},
//	        propagation.TraceContext{},
//	    )),
//	    tracerProviderB,
//	    meterProviderB,
//	    loggerProviderB,
//	    myRuntimeBuilder,
//	)
//
// # Exporters
//
// The package includes subpackages for common exporter configurations:
//
//   - otel/otlp: OTLP exporters for gRPC and HTTP protocols
//   - otel/stdout: Stdout exporters for development and debugging
//   - otel/noop: No-op exporters for testing or disabling telemetry
//
// # Provider Lifecycle
//
// When the Runtime runs, it:
//  1. Registers all providers globally with OpenTelemetry
//  2. Runs the wrapped runtime
//  3. Shuts down all providers when the wrapped runtime completes
//
// Any errors from provider shutdown are joined with the runtime error.
package otel
