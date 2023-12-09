// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package otelconfig provides helpers for initializing specific trace.TracerProviders.
package otelconfig

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

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

// Stdout initializes a TracerProvider which exports to STDOUT.
var Stdout = stdoutIniter{}

type stdoutIniter struct{}

func (stdoutIniter) Init() (trace.TracerProvider, error) {
	exporter, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
	)
	return tp, nil
}
