// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package otelconfig

import (
	"context"
	"slices"
	"time"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
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

	DialOpts []grpc.DialOption

	Compressor string `config:"compressor"`
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

var supportedCompressors = []string{
	"gzip",
}

// OTLPCompressor sets the compressor for the gRPC client to use when sending
// requests. Supported compressor values are: "gzip".
func OTLPCompressor(compressor string) OTLPOption {
	return otlpOptionFunc(func(o *OTLPConfig) {
		o.Compressor = compressor
	})
}

// OTLPDialOptons sets explicit grpc.DialOptions to use when making a connection.
func OTLPDialOptions(opts ...grpc.DialOption) OTLPOption {
	return otlpOptionFunc(func(o *OTLPConfig) {
		o.DialOpts = append(o.DialOpts, opts...)
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
	opts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(cfg.Target),
	}
	if slices.Contains(supportedCompressors, cfg.Compressor) {
		opts = append(opts, otlptracegrpc.WithCompressor(cfg.Compressor))
	}
	if cfg.TransportCredentials == nil {
		opts = append(opts, otlptracegrpc.WithInsecure())
	} else {
		opts = append(opts, otlptracegrpc.WithTLSCredentials(cfg.TransportCredentials))
	}
	if len(cfg.DialOpts) > 0 {
		opts = append(opts, otlptracegrpc.WithDialOption(cfg.DialOpts...))
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Set up a trace exporter
	traceExporter, err := otlptracegrpc.New(ctx, opts...)
	if err != nil {
		return nil, err
	}

	res := cfg.Resource
	if res == nil {
		res, err = resource.New(
			context.Background(),
			resource.WithTelemetrySDK(),
		)
		if err != nil {
			return nil, err
		}
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
