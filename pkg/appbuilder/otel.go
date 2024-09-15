// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package appbuilder

import (
	"context"

	"github.com/z5labs/bedrock"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

type otelOptions struct {
	initPropogator     func(context.Context) (propagation.TextMapPropagator, error)
	initTracerProvider func(context.Context) (trace.TracerProvider, error)
	initMeterProvider  func(context.Context) (metric.MeterProvider, error)
	initLoggerProvider func(context.Context) (log.LoggerProvider, error)
}

// OTelOption
type OTelOption func(*otelOptions)

// OTelTextMapPropogator
func OTelTextMapPropogator(f func(context.Context) (propagation.TextMapPropagator, error)) OTelOption {
	return func(oo *otelOptions) {
		oo.initPropogator = f
	}
}

// OTelTracerProvider
func OTelTracerProvider(f func(context.Context) (trace.TracerProvider, error)) OTelOption {
	return func(oo *otelOptions) {
		oo.initTracerProvider = f
	}
}

// OTelMeterProvider
func OTelMeterProvider(f func(context.Context) (metric.MeterProvider, error)) OTelOption {
	return func(oo *otelOptions) {
		oo.initMeterProvider = f
	}
}

// OTelLoggerProvider
func OTelLoggerProvider(f func(context.Context) (log.LoggerProvider, error)) OTelOption {
	return func(oo *otelOptions) {
		oo.initLoggerProvider = f
	}
}

type builderFunc[T any] func(context.Context, T) (bedrock.App, error)

func (f builderFunc[T]) Build(ctx context.Context, cfg T) (bedrock.App, error) {
	return f(ctx, cfg)
}

// WithOTel
func WithOTel[T any](builder bedrock.AppBuilder[T], opts ...OTelOption) bedrock.AppBuilder[T] {
	oo := &otelOptions{}
	for _, opt := range opts {
		opt(oo)
	}

	return builderFunc[T](func(ctx context.Context, cfg T) (bedrock.App, error) {
		fs := []func(context.Context) error{
			initTextMapPropogator(oo),
			initTracerProvider(oo),
			initMeterProvider(oo),
			initLoggerProvider(oo),
		}

		for _, f := range fs {
			err := f(ctx)
			if err != nil {
				return nil, err
			}
		}

		return builder.Build(ctx, cfg)
	})
}

func initTextMapPropogator(oo *otelOptions) func(context.Context) error {
	return func(ctx context.Context) error {
		if oo.initPropogator == nil {
			return nil
		}

		p, err := oo.initPropogator(ctx)
		if err != nil {
			return err
		}

		otel.SetTextMapPropagator(p)
		return nil
	}
}

func initTracerProvider(oo *otelOptions) func(context.Context) error {
	return func(ctx context.Context) error {
		if oo.initTracerProvider == nil {
			return nil
		}

		tp, err := oo.initTracerProvider(ctx)
		if err != nil {
			return err
		}

		otel.SetTracerProvider(tp)
		return nil
	}
}

func initMeterProvider(oo *otelOptions) func(context.Context) error {
	return func(ctx context.Context) error {
		if oo.initMeterProvider == nil {
			return nil
		}

		mp, err := oo.initMeterProvider(ctx)
		if err != nil {
			return err
		}

		otel.SetMeterProvider(mp)
		return nil
	}
}

func initLoggerProvider(oo *otelOptions) func(context.Context) error {
	return func(ctx context.Context) error {
		if oo.initLoggerProvider == nil {
			return nil
		}

		lp, err := oo.initLoggerProvider(ctx)
		if err != nil {
			return err
		}

		global.SetLoggerProvider(lp)
		return nil
	}
}
