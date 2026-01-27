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

type SpanExporter struct{}

func BuildSpanExporter() bedrock.BuilderFunc[SpanExporter] {
	return func(ctx context.Context) (SpanExporter, error) {
		return SpanExporter{}, nil
	}
}

func (e SpanExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	return nil
}

func (e SpanExporter) Shutdown(ctx context.Context) error {
	return nil
}

type MetricExporter struct{}

func BuildMetricExporter() bedrock.BuilderFunc[MetricExporter] {
	return func(ctx context.Context) (MetricExporter, error) {
		return MetricExporter{}, nil
	}
}

func (e MetricExporter) Temporality(kind sdkmetric.InstrumentKind) metricdata.Temporality {
	return metricdata.CumulativeTemporality
}

func (e MetricExporter) Aggregation(kind sdkmetric.InstrumentKind) sdkmetric.Aggregation {
	return sdkmetric.DefaultAggregationSelector(kind)
}

func (e MetricExporter) Export(ctx context.Context, rm *metricdata.ResourceMetrics) error {
	return nil
}

func (e MetricExporter) ForceFlush(ctx context.Context) error {
	return nil
}

func (e MetricExporter) Shutdown(ctx context.Context) error {
	return nil
}

type LogExporter struct{}

func BuildLogExporter() bedrock.BuilderFunc[LogExporter] {
	return func(ctx context.Context) (LogExporter, error) {
		return LogExporter{}, nil
	}
}

func (e LogExporter) Export(ctx context.Context, records []sdklog.Record) error {
	return nil
}

func (e LogExporter) Shutdown(ctx context.Context) error {
	return nil
}

func (e LogExporter) ForceFlush(ctx context.Context) error {
	return nil
}
