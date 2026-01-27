// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package otel

import (
	"context"
	"fmt"
	"os"

	"github.com/z5labs/bedrock"
	"github.com/z5labs/bedrock/config"
	"github.com/z5labs/bedrock/runtime/otel/stdout"

	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
)

func Example() {
	resourceB := bedrock.MemoizeBuilder(bedrock.BuilderFunc[*resource.Resource](func(ctx context.Context) (*resource.Resource, error) {
		res, err := resource.New(
			ctx,
			resource.WithAttributes(
			// add attributes here
			),
		)
		if err != nil {
			return nil, err
		}

		return res, nil
	}))

	// STDOUT should only be used for example purposes. In production, use OTLP or other exporters.
	stdoutB := bedrock.MemoizeBuilder(bedrock.BuilderOf(os.Stdout))

	tracerProviderB := BuildTracerProvider(
		resourceB,
		BuildTraceIDRatioBasedSampler(
			config.ReaderOf(1.0),
		),
		BuildBatchSpanProcessor(
			stdout.BuildSpanExporter(stdoutB),
		),
	)

	meterProviderB := BuildMeterProvider(
		resourceB,
		BuildPeriodicReader(
			stdout.BuildMetricExporter(stdoutB),
		),
	)

	loggerProviderB := BuildLoggerProvider(
		resourceB,
		BuildBatchLogProcessor(
			stdout.BuildLogExporter(stdoutB),
		),
	)

	runtimeB := BuildRuntime(
		bedrock.BuilderOf(propagation.NewCompositeTextMapPropagator(
			propagation.Baggage{},
			propagation.TraceContext{},
		)),
		tracerProviderB,
		meterProviderB,
		loggerProviderB,
		bedrock.BuilderOf(bedrock.RuntimeFunc(func(ctx context.Context) error {
			fmt.Println("hello from runtime")
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
	// hello from runtime
}
