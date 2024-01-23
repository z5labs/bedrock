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
	"go.opentelemetry.io/otel/trace"
)

// Common is the config for configuring any Initializer.
type Common struct {
	Resource *resource.Resource
}

// CommonOption are options which can be used to configure any Initializer.
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

// Resource configures the resource to be attached to each trace.
func Resource(r *resource.Resource) CommonOption {
	return commonOptionFunc(func(c *Common) {
		c.Resource = r
	})
}

// Initializer represents anything that can initialize a OTel TracerProvider.
type Initializer interface {
	Init() (trace.TracerProvider, error)
}

// Noop is a no-op Initializer which can, for example, be used as
// a default during local development.
var Noop = noopConfiger{}

type noopConfiger struct{}

func (noopConfiger) Init() (trace.TracerProvider, error) {
	return otel.GetTracerProvider(), nil
}

// LocalConfig is the config for the Local Initializer.
type LocalConfig struct {
	Common

	Out io.Writer
}

// LocalOption are options for the Local Initializer.
type LocalOption interface {
	ApplyLocal(*LocalConfig)
}

// Local returns an Initializer for exporting traces to STDOUT.
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

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	return tp, nil
}
