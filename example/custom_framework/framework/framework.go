package framework

import (
	"bytes"
	"context"
	_ "embed"
	"io"
	"log/slog"
	"net/http"
	"os"

	"github.com/z5labs/bedrock"
	bdhttp "github.com/z5labs/bedrock/http"
	"github.com/z5labs/bedrock/pkg/health"
	"github.com/z5labs/bedrock/pkg/lifecycle"
	"github.com/z5labs/bedrock/pkg/noop"
	"github.com/z5labs/bedrock/pkg/otelconfig"
	"github.com/z5labs/bedrock/pkg/otelslog"

	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

//go:embed base_config.yaml
var baseCfgSrc []byte

type Config struct {
	OTel struct {
		ServiceName string `config:"serviceName"`
		OTLP        struct {
			Target string `config:"target"`
		} `config:"otlp"`
	} `config:"otel"`

	Logging struct {
		Level slog.Level `config:"level"`
	} `config:"logging"`
}

var logHandler slog.Handler = noop.LogHandler{}

func initLogger() func(*bedrock.Lifecycle) {
	return func(l *bedrock.Lifecycle) {
		l.PreBuild(func(ctx context.Context) error {
			var cfg Config
			err := UnmarshalConfigFromContext(ctx, &cfg)
			if err != nil {
				return err
			}
			logHandler = otelslog.NewHandler(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
				AddSource: true,
			}))
			return nil
		})
	}
}

func LogHandler() slog.Handler {
	return logHandler
}

func UnmarshalConfigFromContext(ctx context.Context, v interface{}) error {
	m := bedrock.ConfigFromContext(ctx)
	return m.Unmarshal(v)
}

func Rest(cfg io.Reader, f func(context.Context) (http.Handler, error)) error {
	return bedrock.
		New(
			bedrock.Config(bytes.NewReader(baseCfgSrc)),
			bedrock.Config(cfg),
			bedrock.Hooks(
				initLogger(),
				lifecycle.ManageOTel(func(ctx context.Context) (otelconfig.Initializer, error) {
					var cfg Config
					err := UnmarshalConfigFromContext(ctx, &cfg)
					if err != nil {
						return nil, err
					}

					res, err := resource.New(
						context.Background(),
						resource.WithAttributes(
							semconv.ServiceName(cfg.OTel.ServiceName),
						),
					)
					if err != nil {
						return nil, err
					}

					if cfg.OTel.OTLP.Target == "" {
						return otelconfig.Local(
							otelconfig.Resource(res),
						), nil
					}
					return otelconfig.OTLP(
						otelconfig.Resource(res),
						otelconfig.OTLPTarget(cfg.OTel.OTLP.Target),
					), nil
				}),
			),
			bedrock.WithRuntimeBuilderFunc(func(ctx context.Context) (bedrock.Runtime, error) {
				var cfg Config
				err := UnmarshalConfigFromContext(ctx, &cfg)
				if err != nil {
					return nil, err
				}

				h, err := f(ctx)
				if err != nil {
					return nil, err
				}

				rt := bdhttp.NewRuntime(
					bdhttp.Handle("/", h),
					bdhttp.LogHandler(LogHandler()),
					bdhttp.ListenOnPort(8080),
					bdhttp.Liveness(&health.Liveness{}),
					bdhttp.Readiness(&health.Readiness{}),
				)
				return rt, nil
			}),
		).
		Run()
}
