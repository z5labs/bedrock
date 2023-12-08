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
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
)

type GoogleCloudOption func(*gcpIniter)

func ProjectId(id string) GoogleCloudOption {
	return func(gc *gcpIniter) {
		gc.projectId = id
	}
}

func ServiceName(name string) GoogleCloudOption {
	return func(gc *gcpIniter) {
		gc.serviceName = name
	}
}

type gcpIniter struct {
	projectId   string
	serviceName string
}

func GoogleCloud(opts ...GoogleCloudOption) Initializer {
	gc := gcpIniter{}
	for _, opt := range opts {
		opt(&gc)
	}
	return gc
}

func (c gcpIniter) Init() (trace.TracerProvider, error) {
	exporter, err := texporter.New(texporter.WithProjectID(c.projectId))
	if err != nil {
		return nil, err
	}

	// Identify your application using resource detection
	res, err := resource.New(
		context.Background(),
		// Use the GCP resource detector to detect information about the GCP platform
		resource.WithDetectors(gcp.NewDetector()),
		// Keep the default detectors
		resource.WithTelemetrySDK(),
		// Add your own custom attributes to identify your application
		resource.WithAttributes(
			semconv.ServiceNameKey.String(c.serviceName),
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
