// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package otlp

import (
	"context"
	"net/http"

	"github.com/z5labs/bedrock"
	"github.com/z5labs/bedrock/config"

	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"google.golang.org/grpc"
)

func BuildGrpcSpanExporter(grpcConnB bedrock.Builder[*grpc.ClientConn]) bedrock.BuilderFunc[*otlptrace.Exporter] {
	return func(ctx context.Context) (*otlptrace.Exporter, error) {
		return otlptracegrpc.New(
			ctx,
			otlptracegrpc.WithGRPCConn(bedrock.MustBuild(ctx, grpcConnB)),
		)
	}
}

func BuildHttpSpanExporter(
	endpoint config.Reader[string],
	httpClientB bedrock.Builder[*http.Client],
) bedrock.BuilderFunc[*otlptrace.Exporter] {
	return func(ctx context.Context) (*otlptrace.Exporter, error) {
		return otlptracehttp.New(
			ctx,
			otlptracehttp.WithEndpoint(config.Must(ctx, endpoint)),
			otlptracehttp.WithHTTPClient(bedrock.MustBuild(ctx, httpClientB)),
		)
	}
}

func BuildGrpcMetricExporter(grpcConnB bedrock.Builder[*grpc.ClientConn]) bedrock.BuilderFunc[*otlpmetricgrpc.Exporter] {
	return func(ctx context.Context) (*otlpmetricgrpc.Exporter, error) {
		return otlpmetricgrpc.New(
			ctx,
			otlpmetricgrpc.WithGRPCConn(bedrock.MustBuild(ctx, grpcConnB)),
		)
	}
}

func BuildHttpMetricExporter(
	endpoint config.Reader[string],
	httpClientB bedrock.Builder[*http.Client],
) bedrock.BuilderFunc[*otlpmetrichttp.Exporter] {
	return func(ctx context.Context) (*otlpmetrichttp.Exporter, error) {
		return otlpmetrichttp.New(
			ctx,
			otlpmetrichttp.WithEndpoint(config.Must(ctx, endpoint)),
			otlpmetrichttp.WithHTTPClient(bedrock.MustBuild(ctx, httpClientB)),
		)
	}
}

func BuildGrpcLogExporter(grpcConnB bedrock.Builder[*grpc.ClientConn]) bedrock.BuilderFunc[*otlploggrpc.Exporter] {
	return func(ctx context.Context) (*otlploggrpc.Exporter, error) {
		return otlploggrpc.New(
			ctx,
			otlploggrpc.WithGRPCConn(bedrock.MustBuild(ctx, grpcConnB)),
		)
	}
}

func BuildHttpLogExporter(
	endpoint config.Reader[string],
	httpClientB bedrock.Builder[*http.Client],
) bedrock.BuilderFunc[*otlploghttp.Exporter] {
	return func(ctx context.Context) (*otlploghttp.Exporter, error) {
		return otlploghttp.New(
			ctx,
			otlploghttp.WithEndpoint(config.Must(ctx, endpoint)),
			otlploghttp.WithHTTPClient(bedrock.MustBuild(ctx, httpClientB)),
		)
	}
}
