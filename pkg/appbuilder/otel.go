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

// TextMapPropagatorInitializer
type TextMapPropagatorInitializer interface {
	InitTextMapPropogator(context.Context) (propagation.TextMapPropagator, error)
}

// TracerProviderInitializer
type TracerProviderInitializer interface {
	InitTracerProvider(context.Context) (trace.TracerProvider, error)
}

// MeterProviderInitializer
type MeterProviderInitializer interface {
	InitMeterProvider(context.Context) (metric.MeterProvider, error)
}

// LoggerProviderInitializer
type LoggerProviderInitializer interface {
	InitLoggerProvider(context.Context) (log.LoggerProvider, error)
}

// OTelInitializer
type OTelInitializer interface {
	TextMapPropagatorInitializer
	TracerProviderInitializer
	MeterProviderInitializer
	LoggerProviderInitializer
}

// OTel
func OTel[T OTelInitializer](builder bedrock.AppBuilder[T]) bedrock.AppBuilder[T] {
	return bedrock.AppBuilderFunc[T](func(ctx context.Context, cfg T) (bedrock.App, error) {
		fs := []func(context.Context) error{
			func(ctx context.Context) error {
				tmp, err := cfg.InitTextMapPropogator(ctx)
				if err != nil || tmp == nil {
					return err
				}
				otel.SetTextMapPropagator(tmp)
				return nil
			},
			func(ctx context.Context) error {
				tp, err := cfg.InitTracerProvider(ctx)
				if err != nil || tp == nil {
					return err
				}
				otel.SetTracerProvider(tp)
				return nil
			},
			func(ctx context.Context) error {
				mp, err := cfg.InitMeterProvider(ctx)
				if err != nil || mp == nil {
					return err
				}
				otel.SetMeterProvider(mp)
				return nil
			},
			func(ctx context.Context) error {
				lp, err := cfg.InitLoggerProvider(ctx)
				if err != nil || lp == nil {
					return err
				}
				global.SetLoggerProvider(lp)
				return nil
			},
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
