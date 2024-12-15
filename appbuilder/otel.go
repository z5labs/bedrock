// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package appbuilder

import (
	"context"

	"github.com/z5labs/bedrock"
	"github.com/z5labs/bedrock/app"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/log/global"
)

// OTelInitializer represents anything which can initialize the OTel SDK.
type OTelInitializer interface {
	InitializeOTel(context.Context) error
}

// OTel is a [bedrock.AppBuilder] middleware which initializes the OTel SDK.
// It also ensures that the OTel SDK is properly shutdown when the built [bedrock.App]
// stops running.
func OTel[T OTelInitializer](builder bedrock.AppBuilder[T]) bedrock.AppBuilder[T] {
	return bedrock.AppBuilderFunc[T](func(ctx context.Context, cfg T) (bedrock.App, error) {
		err := cfg.InitializeOTel(ctx)
		if err != nil {
			return nil, err
		}

		base, err := builder.Build(ctx, cfg)
		if err != nil {
			return nil, err
		}

		base = app.WithLifecycleHooks(base, app.Lifecycle{
			PostRun: app.ComposeLifecycleHooks(
				tryShutdown(otel.GetTracerProvider()),
				tryShutdown(otel.GetMeterProvider()),
				tryShutdown(global.GetLoggerProvider()),
			),
		})
		return base, nil
	})
}

type shutdowner interface {
	Shutdown(context.Context) error
}

func tryShutdown(v any) app.LifecycleHookFunc {
	return func(ctx context.Context) error {
		if v == nil {
			return nil
		}

		s, ok := v.(shutdowner)
		if !ok {
			return nil
		}
		return s.Shutdown(ctx)
	}
}
