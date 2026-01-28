// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package noop

import (
	"context"

	"github.com/z5labs/bedrock"

	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// SpanExporter is a no-op implementation of the OpenTelemetry SpanExporter interface.
// It discards all exported spans without performing any action.
type SpanExporter struct{}

// BuildSpanExporter returns a Builder that creates a no-op SpanExporter.
func BuildSpanExporter() bedrock.BuilderFunc[SpanExporter] {
	return func(ctx context.Context) (SpanExporter, error) {
		return SpanExporter{}, nil
	}
}

// ExportSpans discards the provided spans and returns nil.
func (e SpanExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	return nil
}

// Shutdown performs no action and returns nil.
func (e SpanExporter) Shutdown(ctx context.Context) error {
	return nil
}

// MetricExporter is a no-op implementation of the OpenTelemetry metric Exporter interface.
// It discards all exported metrics without performing any action.
type MetricExporter struct{}

// BuildMetricExporter returns a Builder that creates a no-op MetricExporter.
func BuildMetricExporter() bedrock.BuilderFunc[MetricExporter] {
	return func(ctx context.Context) (MetricExporter, error) {
		return MetricExporter{}, nil
	}
}

// Temporality returns CumulativeTemporality for all instrument kinds.
func (e MetricExporter) Temporality(kind sdkmetric.InstrumentKind) metricdata.Temporality {
	return metricdata.CumulativeTemporality
}

// Aggregation returns the default aggregation for the given instrument kind.
func (e MetricExporter) Aggregation(kind sdkmetric.InstrumentKind) sdkmetric.Aggregation {
	return sdkmetric.DefaultAggregationSelector(kind)
}

// Export discards the provided metrics and returns nil.
func (e MetricExporter) Export(ctx context.Context, rm *metricdata.ResourceMetrics) error {
	return nil
}

// ForceFlush performs no action and returns nil.
func (e MetricExporter) ForceFlush(ctx context.Context) error {
	return nil
}

// Shutdown performs no action and returns nil.
func (e MetricExporter) Shutdown(ctx context.Context) error {
	return nil
}

// LogExporter is a no-op implementation of the OpenTelemetry log Exporter interface.
// It discards all exported log records without performing any action.
type LogExporter struct{}

// BuildLogExporter returns a Builder that creates a no-op LogExporter.
func BuildLogExporter() bedrock.BuilderFunc[LogExporter] {
	return func(ctx context.Context) (LogExporter, error) {
		return LogExporter{}, nil
	}
}

// Export discards the provided log records and returns nil.
func (e LogExporter) Export(ctx context.Context, records []sdklog.Record) error {
	return nil
}

// Shutdown performs no action and returns nil.
func (e LogExporter) Shutdown(ctx context.Context) error {
	return nil
}

// ForceFlush performs no action and returns nil.
func (e LogExporter) ForceFlush(ctx context.Context) error {
	return nil
}
