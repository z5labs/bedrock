// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package otelconfig

import (
	"context"

	texporter "github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/trace"
	"go.opentelemetry.io/contrib/detectors/gcp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
)

// GoogleCloudConfig
type GoogleCloudConfig struct {
	Common

	ProjectId string `config:"projectId"`
}

// GoogleCloudOption
type GoogleCloudOption interface {
	ApplyGCP(*GoogleCloudConfig)
}

type gcpOptionFunc func(*GoogleCloudConfig)

func (f gcpOptionFunc) ApplyGCP(cfg *GoogleCloudConfig) {
	f(cfg)
}

// ProjectId
func ProjectId(id string) GoogleCloudOption {
	return gcpOptionFunc(func(gcc *GoogleCloudConfig) {
		gcc.ProjectId = id
	})
}

// GoogleCloud
func GoogleCloud(opts ...GoogleCloudOption) Initializer {
	gc := GoogleCloudConfig{}
	for _, opt := range opts {
		opt.ApplyGCP(&gc)
	}
	return gc
}

// Init implements the Initializer interface.
func (cfg GoogleCloudConfig) Init() (trace.TracerProvider, error) {
	exporter, err := texporter.New(texporter.WithProjectID(cfg.ProjectId))
	if err != nil {
		return nil, err
	}

	res, err := resource.New(
		context.Background(),
		resource.WithDetectors(gcp.NewDetector()),
		resource.WithTelemetrySDK(),
		resource.WithAttributes(
			semconv.ServiceName(cfg.Common.ServiceName),
		),
	)
	if err != nil {
		return nil, err
	}

	// Create trace provider with the exporter.
	//
	// By default it uses AlwaysSample() which samples all traces.
	// In a production environment or high QPS setup please use
	// probabilistic sampling.
	// Example:
	//   tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.TraceIDRatioBased(0.0001)), ...)
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	return tp, nil
}
