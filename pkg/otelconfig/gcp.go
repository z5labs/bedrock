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
	"go.opentelemetry.io/otel/trace"
)

// GoogleCloudConfig is the config for the Google Cloud Initializer.
type GoogleCloudConfig struct {
	Common

	ProjectId string `config:"projectId"`
}

// GoogleCloudOption are options for the Google Cloud Initializer.
type GoogleCloudOption interface {
	ApplyGCP(*GoogleCloudConfig)
}

type gcpOptionFunc func(*GoogleCloudConfig)

func (f gcpOptionFunc) ApplyGCP(cfg *GoogleCloudConfig) {
	f(cfg)
}

// GoogleCloudProjectId configures the Google Cloud Project ID.
func GoogleCloudProjectId(id string) GoogleCloudOption {
	return gcpOptionFunc(func(gcc *GoogleCloudConfig) {
		gcc.ProjectId = id
	})
}

// GoogleCloud returns an Initializer for exporting traces directly to Cloud Trace.
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

	res := cfg.Resource
	if res == nil {
		res, err = resource.New(
			context.Background(),
			resource.WithDetectors(gcp.NewDetector()),
			resource.WithTelemetrySDK(),
		)
		if err != nil {
			return nil, err
		}
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
