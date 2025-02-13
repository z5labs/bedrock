// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package appbuilder

import (
	"context"
	"errors"

	"github.com/z5labs/bedrock"
	"github.com/z5labs/bedrock/app"
	"github.com/z5labs/bedrock/lifecycle"
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
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		err := cfg.InitializeOTel(ctx)
		if err != nil {
			return nil, err
		}

		onPostRun := lifecycle.MultiHook(
			tryShutdown(otel.GetTracerProvider()),
			tryShutdown(otel.GetMeterProvider()),
			tryShutdown(global.GetLoggerProvider()),
		)

		base, err := builder.Build(ctx, cfg)
		if err != nil {
			shutdownErr := onPostRun.Run(ctx)
			if shutdownErr == nil {
				return nil, err
			}
			return nil, errors.Join(err, shutdownErr)
		}

		lc, ok := lifecycle.FromContext(ctx)
		if !ok {
			base = app.PostRun(base, onPostRun)
			return base, nil
		}

		lc.OnPostRun(onPostRun)
		return base, nil
	})
}

type shutdowner interface {
	Shutdown(context.Context) error
}

func tryShutdown(v any) lifecycle.HookFunc {
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
