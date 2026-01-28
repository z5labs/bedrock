// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package noop

import (
	"context"
	"fmt"

	"github.com/z5labs/bedrock"
	"github.com/z5labs/bedrock/runtime/otel"

	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
)

func Example() {
	resourceB := bedrock.MemoizeBuilder(bedrock.BuilderFunc[*resource.Resource](func(ctx context.Context) (*resource.Resource, error) {
		return resource.Default(), nil
	}))

	tracerProviderB := otel.BuildTracerProvider(
		resourceB,
		bedrock.BuilderOf(trace.AlwaysSample()),
		otel.BuildBatchSpanProcessor(
			BuildSpanExporter(),
		),
	)

	meterProviderB := otel.BuildMeterProvider(
		resourceB,
		otel.BuildPeriodicReader(
			BuildMetricExporter(),
		),
	)

	loggerProviderB := otel.BuildLoggerProvider(
		resourceB,
		otel.BuildBatchLogProcessor(
			BuildLogExporter(),
		),
	)

	runtimeB := otel.BuildRuntime(
		bedrock.BuilderOf(propagation.NewCompositeTextMapPropagator(
			propagation.Baggage{},
			propagation.TraceContext{},
		)),
		tracerProviderB,
		meterProviderB,
		loggerProviderB,
		bedrock.BuilderOf(bedrock.RuntimeFunc(func(ctx context.Context) error {
			fmt.Println("hello from noop runtime")
			return nil
		})),
	)

	rt, err := runtimeB.Build(context.Background())
	if err != nil {
		fmt.Println(err)
		return
	}

	if err := rt.Run(context.Background()); err != nil {
		fmt.Println(err)
		return
	}

	// Output:
	// hello from noop runtime
}
