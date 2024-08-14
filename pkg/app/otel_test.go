// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package app

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/log"
	lognoop "go.opentelemetry.io/otel/log/noop"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

func TestWithOTel(t *testing.T) {
	t.Run("will return an error", func(t *testing.T) {
		t.Run("if the base bedrock.App fails to run", func(t *testing.T) {
			baseErr := errors.New("failed to run")
			base := runFunc(func(ctx context.Context) error {
				return baseErr
			})

			app := WithOTel(base)
			err := app.Run(context.Background())
			if !assert.ErrorIs(t, err, baseErr) {
				return
			}
		})

		t.Run("if propagation.TextMapPropagator fails to initialize", func(t *testing.T) {
			base := runFunc(func(ctx context.Context) error {
				return nil
			})

			initErr := errors.New("failed to init")
			app := WithOTel(base, OTelTextMapPropogator(func(ctx context.Context) (propagation.TextMapPropagator, error) {
				return nil, initErr
			}))

			err := app.Run(context.Background())
			if !assert.ErrorIs(t, err, initErr) {
				return
			}
		})

		t.Run("if trace.TracerProvider fails to initialize", func(t *testing.T) {
			base := runFunc(func(ctx context.Context) error {
				return nil
			})

			initErr := errors.New("failed to init")
			app := WithOTel(base, OTelTracerProvider(func(ctx context.Context) (trace.TracerProvider, error) {
				return nil, initErr
			}))

			err := app.Run(context.Background())
			if !assert.ErrorIs(t, err, initErr) {
				return
			}
		})

		t.Run("if metric.MeterProvider fails to initialize", func(t *testing.T) {
			base := runFunc(func(ctx context.Context) error {
				return nil
			})

			initErr := errors.New("failed to init")
			app := WithOTel(base, OTelMeterProvider(func(ctx context.Context) (metric.MeterProvider, error) {
				return nil, initErr
			}))

			err := app.Run(context.Background())
			if !assert.ErrorIs(t, err, initErr) {
				return
			}
		})

		t.Run("if log.LoggerProvider fails to initialize", func(t *testing.T) {
			base := runFunc(func(ctx context.Context) error {
				return nil
			})

			initErr := errors.New("failed to init")
			app := WithOTel(base, OTelLoggerProvider(func(ctx context.Context) (log.LoggerProvider, error) {
				return nil, initErr
			}))

			err := app.Run(context.Background())
			if !assert.ErrorIs(t, err, initErr) {
				return
			}
		})
	})

	t.Run("will not return an error", func(t *testing.T) {
		t.Run("if propagation.TextMapPropagator succeeds to initialize", func(t *testing.T) {
			base := runFunc(func(ctx context.Context) error {
				return nil
			})

			app := WithOTel(base, OTelTextMapPropogator(func(ctx context.Context) (propagation.TextMapPropagator, error) {
				return propagation.TraceContext{}, nil
			}))

			err := app.Run(context.Background())
			if !assert.Nil(t, err) {
				return
			}
		})

		t.Run("if trace.TracerProvider succeeds to initialize", func(t *testing.T) {
			base := runFunc(func(ctx context.Context) error {
				return nil
			})

			app := WithOTel(base, OTelTracerProvider(func(ctx context.Context) (trace.TracerProvider, error) {
				return tracenoop.NewTracerProvider(), nil
			}))

			err := app.Run(context.Background())
			if !assert.Nil(t, err) {
				return
			}
		})

		t.Run("if metric.MeterProvider succeeds to initialize", func(t *testing.T) {
			base := runFunc(func(ctx context.Context) error {
				return nil
			})

			app := WithOTel(base, OTelMeterProvider(func(ctx context.Context) (metric.MeterProvider, error) {
				return metricnoop.NewMeterProvider(), nil
			}))

			err := app.Run(context.Background())
			if !assert.Nil(t, err) {
				return
			}
		})

		t.Run("if log.LoggerProvider succeeds to initialize", func(t *testing.T) {
			base := runFunc(func(ctx context.Context) error {
				return nil
			})

			app := WithOTel(base, OTelLoggerProvider(func(ctx context.Context) (log.LoggerProvider, error) {
				return lognoop.NewLoggerProvider(), nil
			}))

			err := app.Run(context.Background())
			if !assert.Nil(t, err) {
				return
			}
		})
	})
}
