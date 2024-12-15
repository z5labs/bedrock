// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/z5labs/bedrock"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutlog"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func ExampleWithOTel_textMapPropogator() {
	carrier := make(propagation.MapCarrier)
	var app bedrock.App = runFunc(func(ctx context.Context) error {
		tmp := otel.GetTextMapPropagator()
		tmp.Inject(ctx, carrier)
		return nil
	})

	app = WithOTel(
		app,
		OTelTextMapPropogator(func(ctx context.Context) (propagation.TextMapPropagator, error) {
			tmp := propagation.Baggage{}
			return tmp, nil
		}),
	)

	m, _ := baggage.NewMember("hello", "world")
	b, _ := baggage.New(m)
	ctx := baggage.ContextWithBaggage(context.Background(), b)

	err := app.Run(ctx)
	if err != nil {
		fmt.Println(err)
		return
	}

	ctx = propagation.Baggage{}.Extract(context.Background(), carrier)
	b = baggage.FromContext(ctx)
	m = b.Member("hello")
	fmt.Println(m.Value())
	// Output: world
}

func ExampleWithOTel_tracerProvider() {
	var app bedrock.App = runFunc(func(ctx context.Context) error {
		_, span := otel.Tracer("app").Start(ctx, "Run")
		defer span.End()
		return nil
	})

	var tp *sdktrace.TracerProvider
	var buf bytes.Buffer
	app = WithOTel(
		app,
		OTelTracerProvider(func(ctx context.Context) (trace.TracerProvider, error) {
			// NOTE: this is only for example purposes. DO NOT USE IN PRODUCTION!!!
			exp, err := stdouttrace.New(
				stdouttrace.WithWriter(&buf),
			)
			if err != nil {
				return nil, err
			}

			sp := sdktrace.NewSimpleSpanProcessor(exp)

			tp = sdktrace.NewTracerProvider(
				sdktrace.WithSpanProcessor(sp),
			)
			return tp, nil
		}),
	)

	err := app.Run(context.Background())
	if err != nil {
		fmt.Println(err)
		return
	}

	// Ensure that the app trace is flushed to buf
	err = tp.Shutdown(context.Background())
	if err != nil {
		fmt.Println(err)
		return
	}

	b, err := io.ReadAll(&buf)
	if err != nil {
		fmt.Println(err)
		return
	}

	var m map[string]any
	err = json.Unmarshal(b, &m)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(m["Name"])
	// Output: Run
}

func ExampleWithOTel_meterProvider() {
	var app bedrock.App = runFunc(func(ctx context.Context) error {
		counter, err := otel.Meter("app").Int64Counter("Run")
		if err != nil {
			return err
		}
		counter.Add(ctx, 1)
		return nil
	})

	var mp *sdkmetric.MeterProvider
	var buf bytes.Buffer
	app = WithOTel(
		app,
		OTelMeterProvider(func(ctx context.Context) (metric.MeterProvider, error) {
			// NOTE: this is only for example purposes. DO NOT USE IN PRODUCTION!!!
			exp, err := stdoutmetric.New(
				stdoutmetric.WithWriter(&buf),
			)
			if err != nil {
				return nil, err
			}

			r := sdkmetric.NewPeriodicReader(exp)

			mp = sdkmetric.NewMeterProvider(
				sdkmetric.WithReader(r),
			)
			return mp, nil
		}),
	)

	err := app.Run(context.Background())
	if err != nil {
		fmt.Println(err)
		return
	}

	// Ensure that the app metric is flushed to buf
	err = mp.Shutdown(context.Background())
	if err != nil {
		fmt.Println(err)
		return
	}

	b, err := io.ReadAll(&buf)
	if err != nil {
		fmt.Println(err)
		return
	}

	var m struct {
		ScopeMetrics []struct {
			Metrics []struct {
				Name string `json:"Name"`
				Data struct {
					DataPoints []struct {
						Value int `json:"Value"`
					} `json:"DataPoints"`
				} `json:"Data"`
			} `json:"Metrics"`
		} `json:"ScopeMetrics"`
	}
	err = json.Unmarshal(b, &m)
	if err != nil {
		fmt.Println(err)
		return
	}

	metric := m.ScopeMetrics[0].Metrics[0]
	fmt.Println(metric.Name, metric.Data.DataPoints[0].Value)
	// Output: Run 1
}

func ExampleWithOTel_loggerProvider() {
	var app bedrock.App = runFunc(func(ctx context.Context) error {
		// here we're using the otelslog bridge which will use the global
		// LoggerProvider for us to create a otel Logger and map between
		// the slog and otel log record types.
		logger := otelslog.NewLogger("app")
		logger.InfoContext(ctx, "hello")
		return nil
	})

	var lp *sdklog.LoggerProvider
	var buf bytes.Buffer
	app = WithOTel(
		app,
		OTelLoggerProvider(func(ctx context.Context) (log.LoggerProvider, error) {
			// NOTE: this is only for example purposes. DO NOT USE IN PRODUCTION!!!
			exp, err := stdoutlog.New(
				stdoutlog.WithWriter(&buf),
			)
			if err != nil {
				return nil, err
			}

			p := sdklog.NewSimpleProcessor(exp)

			lp = sdklog.NewLoggerProvider(
				sdklog.WithProcessor(p),
			)
			return lp, nil
		}),
	)

	err := app.Run(context.Background())
	if err != nil {
		fmt.Println(err)
		return
	}

	// Ensure that the app log is flushed to buf
	err = lp.Shutdown(context.Background())
	if err != nil {
		fmt.Println(err)
		return
	}

	b, err := io.ReadAll(&buf)
	if err != nil {
		fmt.Println(err)
		return
	}

	var m struct {
		Body struct {
			Value string `json:"Value"`
		} `json:"Body"`
		Scope struct {
			Name string `json:"Name"`
		} `json:"Scope"`
	}
	err = json.Unmarshal(b, &m)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(m.Scope.Name, m.Body.Value)
	// Output: app hello
}
