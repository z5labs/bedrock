// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package otelconfig

import (
	"context"
	"io"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
)

// Common
type Common struct {
	ServiceName string `config:"serviceName"`
}

// CommonOption
type CommonOption interface {
	GoogleCloudOption
	LocalOption
	OTLPOption
}

type commonOptionFunc func(*Common)

func (f commonOptionFunc) ApplyGCP(cfg *GoogleCloudConfig) {
	f(&cfg.Common)
}

func (f commonOptionFunc) ApplyOTLP(cfg *OTLPConfig) {
	f(&cfg.Common)
}

func (f commonOptionFunc) ApplyLocal(cfg *LocalConfig) {
	f(&cfg.Common)
}

// ServiceName
func ServiceName(name string) CommonOption {
	return commonOptionFunc(func(c *Common) {
		c.ServiceName = name
	})
}

// Initializer
type Initializer interface {
	Init() (trace.TracerProvider, error)
}

// Noop
var Noop = noopConfiger{}

type noopConfiger struct{}

func (noopConfiger) Init() (trace.TracerProvider, error) {
	return otel.GetTracerProvider(), nil
}

// LocalConfig
type LocalConfig struct {
	Common

	Out io.Writer
}

// LocalOption
type LocalOption interface {
	ApplyLocal(*LocalConfig)
}

// Local
func Local(opts ...LocalOption) Initializer {
	cfg := LocalConfig{
		Out: os.Stdout,
	}
	for _, opt := range opts {
		opt.ApplyLocal(&cfg)
	}
	return cfg
}

// Init implements Initializer interface.
func (cfg LocalConfig) Init() (trace.TracerProvider, error) {
	exporter, err := stdouttrace.New(
		stdouttrace.WithWriter(cfg.Out),
	)
	if err != nil {
		return nil, err
	}

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

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	return tp, nil
}
