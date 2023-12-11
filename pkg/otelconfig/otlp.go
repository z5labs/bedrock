// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package otelconfig

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// OTLPConfig
type OTLPConfig struct {
	Common

	// gRPC traget string which is passed to grpc.Dial()
	Target string `config:"target"`
}

// OTLPOption
type OTLPOption interface {
	ApplyOTLP(*OTLPConfig)
}

// OTLP
func OTLP(opts ...OTLPOption) Initializer {
	c := OTLPConfig{}
	for _, opt := range opts {
		opt.ApplyOTLP(&c)
	}
	return c
}

// Init implements Initializer interface.
func (cfg OTLPConfig) Init() (trace.TracerProvider, error) {
	res, err := resource.New(
		context.Background(),
		resource.WithTelemetrySDK(),
		resource.WithAttributes(
			semconv.ServiceName(cfg.Common.ServiceName),
		),
	)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	conn, err := grpc.DialContext(
		ctx,
		cfg.Target,
		// Note the use of insecure transport here. TLS is recommended in production.
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, err
	}

	// Set up a trace exporter
	traceExporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn))
	if err != nil {
		return nil, err
	}

	// Register the trace exporter with a TracerProvider, using a batch
	// span processor to aggregate spans before export.
	bsp := sdktrace.NewBatchSpanProcessor(traceExporter)
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithResource(res),
		sdktrace.WithSpanProcessor(bsp),
	)
	return tp, nil
}
