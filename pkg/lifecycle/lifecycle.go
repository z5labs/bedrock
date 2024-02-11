// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package lifecycle

import (
	"context"

	"github.com/z5labs/bedrock"
	"github.com/z5labs/bedrock/pkg/otelconfig"

	"go.opentelemetry.io/otel"
)

// ManageOTel is a hook for intializing OTel on PreBuild and shutting it down on PostRun.
func ManageOTel(f func(context.Context) (otelconfig.Initializer, error)) func(*bedrock.Lifecycle) {
	return func(life *bedrock.Lifecycle) {
		life.PreBuild(func(ctx context.Context) error {
			initer, err := f(ctx)
			if err != nil {
				return err
			}
			tp, err := initer.Init()
			if err != nil {
				return err
			}
			otel.SetTracerProvider(tp)
			return nil
		})

		life.PostRun(func(ctx context.Context) error {
			tp := otel.GetTracerProvider()
			stp, ok := tp.(interface {
				Shutdown(context.Context) error
			})
			if !ok {
				return nil
			}
			return stp.Shutdown(ctx)
		})
	}
}
