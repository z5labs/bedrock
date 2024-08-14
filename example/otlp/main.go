// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package main

import (
	"context"
	"embed"
	"encoding/json"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/z5labs/bedrock"
	brhttp "github.com/z5labs/bedrock/http"
	"github.com/z5labs/bedrock/pkg/config"
	"github.com/z5labs/bedrock/pkg/otelconfig"
	"github.com/z5labs/bedrock/pkg/otelslog"
	"github.com/z5labs/bedrock/pkg/slogfield"
	"github.com/z5labs/bedrock/queue"
	"golang.org/x/sync/errgroup"

	"go.opentelemetry.io/otel"
)

type Config struct {
	Logging struct {
		Level slog.Level `config:"level"`
	} `config:"logging"`
}

func initHttpRuntime(ctx context.Context, cfg Config) (bedrock.App, error) {
	logHandler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{AddSource: true})
	logger := otelslog.New(logHandler)

	rt := brhttp.NewRuntime(
		brhttp.ListenOnPort(8080),
		brhttp.LogHandler(logHandler),
		brhttp.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			spanCtx, span := otel.Tracer("main").Start(r.Context(), "handler")
			defer span.End()

			n := rand.Int()
			enc := json.NewEncoder(w)
			err := enc.Encode(struct{ N int }{N: n})
			if err != nil {
				logger.ErrorContext(spanCtx, "failed to encode reponse", slogfield.Error(err))
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}),
	)
	return rt, nil
}

type consumerFunc[T any] func(context.Context) (T, error)

func (f consumerFunc[T]) Consume(ctx context.Context) (T, error) {
	return f(ctx)
}

type processorFunc[T any] func(context.Context, T) error

func (f processorFunc[T]) Process(ctx context.Context, t T) error {
	return f(ctx, t)
}

func initQueueRuntime(ctx context.Context, cfg Config) (bedrock.App, error) {
	logHandler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		AddSource: true,
		Level:     cfg.Logging.Level,
	})
	logger := otelslog.New(logHandler)

	c := consumerFunc[int](func(ctx context.Context) (int, error) {
		spanCtx, span := otel.Tracer("main").Start(ctx, "consumer")
		defer span.End()

		// randomly wait a few seconds so we're not generating
		// a huge amount of logs and traces
		select {
		case <-spanCtx.Done():
			return 0, spanCtx.Err()
		case <-time.After(time.Duration(rand.Intn(5)+1) * time.Second):
		}

		resp, err := http.Get("http://localhost:8080")
		if err != nil {
			logger.ErrorContext(spanCtx, "failed to call http service", slogfield.Error(err))
			return 0, err
		}

		b, err := readAllAndClose(resp.Body)
		if err != nil {
			logger.ErrorContext(spanCtx, "failed to read response body", slogfield.Error(err))
			return 0, err
		}

		var res struct {
			N int
		}
		err = json.Unmarshal(b, &res)
		if err != nil {
			logger.ErrorContext(spanCtx, "failed to unmarshal response body", slogfield.Error(err))
			return 0, err
		}
		logger.InfoContext(spanCtx, "consumed int", slogfield.Int("n", res.N))
		return res.N, nil
	})

	p := processorFunc[int](func(ctx context.Context, n int) error {
		spanCtx, span := otel.Tracer("main").Start(ctx, "processor")
		defer span.End()

		logger.InfoContext(spanCtx, "processing int", slogfield.Int("n", n))
		return nil
	})

	rt := queue.Concurrent[int](
		c,
		p,
		queue.LogHandler(logHandler),
	)
	return rt, nil
}

func otlpOtel(ctx context.Context) (otelconfig.Initializer, error) {
	initer := otelconfig.OTLP(
		otelconfig.OTLPTarget("otlp-opentelemetry-collector:4317"),
	)
	return initer, nil
}

//go:embed config.yaml
var configDir embed.FS

func main() {
	eg, egctx := errgroup.WithContext(context.Background())
	eg.Go(func() error {
		return bedrock.Run(
			egctx,
			bedrock.AppBuilderFunc[Config](initHttpRuntime),
			config.FromYaml(
				config.NewFileReader(configDir, "config.yaml"),
			),
		)
	})
	eg.Go(func() error {
		return bedrock.Run(
			egctx,
			bedrock.AppBuilderFunc[Config](initQueueRuntime),
			config.FromYaml(
				config.NewFileReader(configDir, "config.yaml"),
			),
		)
	})

	err := eg.Wait()
	if err != nil {
		slog.Default().Error("failed to run", slog.String("error", err.Error()))
	}
}

func readAllAndClose(rc io.ReadCloser) ([]byte, error) {
	defer rc.Close()
	return io.ReadAll(rc)
}
