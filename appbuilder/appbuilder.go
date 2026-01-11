// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package appbuilder

import (
	"context"
	"errors"
	"os"
	"os/signal"

	"github.com/z5labs/bedrock"
	"github.com/z5labs/bedrock/app"
	"github.com/z5labs/bedrock/lifecycle"

	"github.com/z5labs/sdk-go/try"
)

// Recover will wrap the given [bedrock.AppBuilder] with panic recovery.
func Recover[T any](builder bedrock.AppBuilder[T]) bedrock.AppBuilder[T] {
	return bedrock.AppBuilderFunc[T](func(ctx context.Context, cfg T) (_ bedrock.App, err error) {
		defer try.Recover(&err)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		return builder.Build(ctx, cfg)
	})
}

// LifecycleContext injects the given [lifecycle.Context] into the build [context.Context]
// and wraps the underlying built [bedrock.App] with the [app.PostRun] middleware so any
// [lifecycle.Hook]s registered with [lifecycle.Context.OnPostRun] will be executed after
// [bedrock.App.Run]. The [lifecycle.Hook]s will also be executed in case the given
// [bedrock.AppBuilder] fails.
func LifecycleContext[T any](builder bedrock.AppBuilder[T], lc *lifecycle.Context) bedrock.AppBuilder[T] {
	return bedrock.AppBuilderFunc[T](func(ctx context.Context, cfg T) (bedrock.App, error) {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		ctx = lifecycle.NewContext(ctx, lc)
		base, err := builder.Build(ctx, cfg)
		if err != nil {
			hook := lc.PostRun()
			hookErr := hook.Run(ctx)
			if hookErr == nil {
				return nil, err
			}
			return nil, errors.Join(err, hookErr)
		}

		base = app.PostRun(base, lc.PostRun())
		return base, nil
	})
}

// InterruptOn wraps a given [bedrock.AppBuilder] in an implementation
// that cancels the [context.Context] that's passed to builder.Build if an [os.Signal]
// is received by the running process. Once builder.Build completes the [os.Signal]
// listening is stopped. Thus, this middleware only applies to the given builder and does
// not wrap the returned [bedrock.App] with signal cancellation. For [bedrock.App] signal
// cancellation, please use the [app.InterruptOn] middleware.
func InterruptOn[T any](builder bedrock.AppBuilder[T], signals ...os.Signal) bedrock.AppBuilder[T] {
	return bedrock.AppBuilderFunc[T](func(ctx context.Context, cfg T) (bedrock.App, error) {
		sigCtx, stop := signal.NotifyContext(ctx, signals...)
		defer stop()

		return builder.Build(sigCtx, cfg)
	})
}
