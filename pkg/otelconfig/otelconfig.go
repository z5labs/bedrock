// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package otelconfig provides helpers for initializing specific trace.TracerProviders.
package otelconfig

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

// Initializer
type Initializer interface {
	Init() (trace.TracerProvider, error)
}

// Noop
var Noop = noopConfiger{}

type noopConfiger struct{}

func (c noopConfiger) Init() (trace.TracerProvider, error) {
	return otel.GetTracerProvider(), nil
}
