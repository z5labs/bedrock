// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package appbuilder

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/z5labs/bedrock"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/log/global"
	lognoop "go.opentelemetry.io/otel/log/noop"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

type failToInitOTel struct{}

var failedToInitOTelErr = errors.New("failed to init otel")

func (failToInitOTel) InitializeOTel(ctx context.Context) error {
	return failedToInitOTelErr
}

type noopInitOTel struct{}

func (noopInitOTel) InitializeOTel(ctx context.Context) error {
	return nil
}

type appFunc func(context.Context) error

func (f appFunc) Run(ctx context.Context) error {
	return f(ctx)
}

type tracerProvider struct {
	tracenoop.TracerProvider
	shutdown func(context.Context) error
}

func newTracerProvider(shutdown func(context.Context) error) tracerProvider {
	return tracerProvider{
		shutdown: shutdown,
	}
}

func (tp tracerProvider) Shutdown(ctx context.Context) error {
	return tp.shutdown(ctx)
}

type tracerProviderInitOTel struct{}

var errTracerProviderFailedShutdown = errors.New("failed to shutdown tracer provider")

func (tracerProviderInitOTel) InitializeOTel(ctx context.Context) error {
	otel.SetTracerProvider(newTracerProvider(func(ctx context.Context) error {
		return errTracerProviderFailedShutdown
	}))
	return nil
}

type meterProvider struct {
	metricnoop.MeterProvider
	shutdown func(context.Context) error
}

func newMeterProvider(shutdown func(context.Context) error) meterProvider {
	return meterProvider{
		shutdown: shutdown,
	}
}

func (mp meterProvider) Shutdown(ctx context.Context) error {
	return mp.shutdown(ctx)
}

type meterProviderInitOTel struct{}

var errMeterProviderFailedShutdown = errors.New("failed to shutdown meter provider")

func (meterProviderInitOTel) InitializeOTel(ctx context.Context) error {
	otel.SetMeterProvider(newMeterProvider(func(ctx context.Context) error {
		return errMeterProviderFailedShutdown
	}))
	return nil
}

type loggerProvider struct {
	lognoop.LoggerProvider
	shutdown func(context.Context) error
}

func newLoggerProvider(shutdown func(context.Context) error) loggerProvider {
	return loggerProvider{
		shutdown: shutdown,
	}
}

func (mp loggerProvider) Shutdown(ctx context.Context) error {
	return mp.shutdown(ctx)
}

type loggerProviderInitOTel struct{}

var errLoggerProviderFailedShutdown = errors.New("failed to shutdown logger provider")

func (loggerProviderInitOTel) InitializeOTel(ctx context.Context) error {
	global.SetLoggerProvider(newLoggerProvider(func(ctx context.Context) error {
		return errLoggerProviderFailedShutdown
	}))
	return nil
}

func TestOTel(t *testing.T) {
	t.Run("bedrock.AppBuilder will return an error", func(t *testing.T) {
		t.Run("if InitializeOTel fails", func(t *testing.T) {
			b := OTel(bedrock.AppBuilderFunc[failToInitOTel](func(ctx context.Context, cfg failToInitOTel) (bedrock.App, error) {
				return nil, nil
			}))

			_, err := b.Build(context.Background(), failToInitOTel{})
			if !assert.ErrorIs(t, err, failedToInitOTelErr) {
				return
			}
		})

		t.Run("if the given bedrock.AppBuilder fails", func(t *testing.T) {
			buildErr := errors.New("failed to build")
			b := OTel(bedrock.AppBuilderFunc[noopInitOTel](func(ctx context.Context, cfg noopInitOTel) (bedrock.App, error) {
				return nil, buildErr
			}))

			_, err := b.Build(context.Background(), noopInitOTel{})
			if !assert.ErrorIs(t, err, buildErr) {
				return
			}
		})
	})

	t.Run("the built bedrock.App will return an error", func(t *testing.T) {
		t.Run("if it fails to shutdown the tracer provider", func(t *testing.T) {
			b := OTel(bedrock.AppBuilderFunc[tracerProviderInitOTel](func(ctx context.Context, cfg tracerProviderInitOTel) (bedrock.App, error) {
				a := appFunc(func(ctx context.Context) error {
					return nil
				})
				return a, nil
			}))

			app, err := b.Build(context.Background(), tracerProviderInitOTel{})
			if !assert.Nil(t, err) {
				return
			}

			err = app.Run(context.Background())
			if !assert.ErrorIs(t, err, errTracerProviderFailedShutdown) {
				return
			}
		})

		t.Run("if it fails to shutdown the meter provider", func(t *testing.T) {
			b := OTel(bedrock.AppBuilderFunc[meterProviderInitOTel](func(ctx context.Context, cfg meterProviderInitOTel) (bedrock.App, error) {
				a := appFunc(func(ctx context.Context) error {
					return nil
				})
				return a, nil
			}))

			app, err := b.Build(context.Background(), meterProviderInitOTel{})
			if !assert.Nil(t, err) {
				return
			}

			err = app.Run(context.Background())
			if !assert.ErrorIs(t, err, errMeterProviderFailedShutdown) {
				return
			}
		})

		t.Run("if it fails to shutdown the logger provider", func(t *testing.T) {
			b := OTel(bedrock.AppBuilderFunc[loggerProviderInitOTel](func(ctx context.Context, cfg loggerProviderInitOTel) (bedrock.App, error) {
				a := appFunc(func(ctx context.Context) error {
					return nil
				})
				return a, nil
			}))

			app, err := b.Build(context.Background(), loggerProviderInitOTel{})
			if !assert.Nil(t, err) {
				return
			}

			err = app.Run(context.Background())
			if !assert.ErrorIs(t, err, errMeterProviderFailedShutdown) {
				return
			}
		})
	})
}
