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
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// OTLPConfig is the config for the OTLP Initializer.
type OTLPConfig struct {
	Common

	// gRPC traget string which is passed to grpc.Dial()
	Target string `config:"target"`

	TransportCredentials credentials.TransportCredentials
}

// OTLPOption are options for the OTLP Initializer.
type OTLPOption interface {
	ApplyOTLP(*OTLPConfig)
}

type otlpOptionFunc func(*OTLPConfig)

func (f otlpOptionFunc) ApplyOTLP(cfg *OTLPConfig) {
	f(cfg)
}

// OTLPTarget configures the gRPC target where traces will be sent to via OTLP.
func OTLPTarget(target string) OTLPOption {
	return otlpOptionFunc(func(o *OTLPConfig) {
		o.Target = target
	})
}

// OTLPTransportCreds configures the gRPC transport credentials for the OTLP target.
func OTLPTransportCreds(tc credentials.TransportCredentials) OTLPOption {
	return otlpOptionFunc(func(o *OTLPConfig) {
		o.TransportCredentials = tc
	})
}

// OTLP returns an Initializer for exporting traces via OTLP.
func OTLP(opts ...OTLPOption) Initializer {
	c := OTLPConfig{
		TransportCredentials: insecure.NewCredentials(),
	}
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
		grpc.WithTransportCredentials(cfg.TransportCredentials),
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
