// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package stdout

import (
	"context"
	"io"

	"github.com/z5labs/bedrock"

	"go.opentelemetry.io/otel/exporters/stdout/stdoutlog"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/metric"
)

func BuildSpanExporter[W io.Writer](writerB bedrock.Builder[W]) bedrock.BuilderFunc[*stdouttrace.Exporter] {
	return func(ctx context.Context) (*stdouttrace.Exporter, error) {
		return stdouttrace.New(
			stdouttrace.WithWriter(bedrock.MustBuild(ctx, writerB)),
		)
	}
}

func BuildMetricExporter[W io.Writer](writerB bedrock.Builder[W]) bedrock.BuilderFunc[metric.Exporter] {
	return func(ctx context.Context) (metric.Exporter, error) {
		return stdoutmetric.New(
			stdoutmetric.WithWriter(bedrock.MustBuild(ctx, writerB)),
		)
	}
}

func BuildLogExporter[W io.Writer](writerB bedrock.Builder[W]) bedrock.BuilderFunc[*stdoutlog.Exporter] {
	return func(ctx context.Context) (*stdoutlog.Exporter, error) {
		return stdoutlog.New(
			stdoutlog.WithWriter(bedrock.MustBuild(ctx, writerB)),
		)
	}
}
