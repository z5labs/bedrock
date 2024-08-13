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
	"github.com/z5labs/bedrock/http/httphealth"
	"github.com/z5labs/bedrock/http/httpvalidate"
	"github.com/z5labs/bedrock/pkg/config"
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

	Http struct {
		Port uint `config:"port"`
	} `config:"http"`
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

func Rest(cfg io.Reader, f func(context.Context, *http.ServeMux) error) error {
	return bedrock.
		New(
			bedrock.Config(
				config.NewYamlSource(
					config.RenderTextTemplate(
						bytes.NewReader(baseCfgSrc),
						config.TemplateFunc("env", os.Getenv),
						config.TemplateFunc("default", func(v any, def string) any {
							if v == nil {
								return def
							}
							return v
						}),
					),
				),
			),
			bedrock.Config(
				config.NewYamlSource(
					config.RenderTextTemplate(
						cfg,
						config.TemplateFunc("env", os.Getenv),
						config.TemplateFunc("default", func(v any, def string) any {
							if v == nil {
								return def
							}
							return v
						}),
					),
				),
			),
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

				mux := http.NewServeMux()
				mux.Handle(
					"/health/liveness",
					httpvalidate.Request(
						httphealth.NewHandler(&health.Binary{}),
						httpvalidate.ForMethods(http.MethodGet),
					),
				)
				mux.Handle(
					"/health/readiness",
					httpvalidate.Request(
						httphealth.NewHandler(&health.Binary{}),
						httpvalidate.ForMethods(http.MethodGet),
					),
				)
				mux.Handle(
					"/health/started",
					httpvalidate.Request(
						httphealth.NewHandler(&health.Binary{}),
						httpvalidate.ForMethods(http.MethodGet),
					),
				)

				err = f(ctx, mux)
				if err != nil {
					return nil, err
				}

				rt := bdhttp.NewRuntime(
					bdhttp.Handle("/", mux),
					bdhttp.LogHandler(LogHandler()),
					bdhttp.ListenOnPort(cfg.Http.Port),
				)
				return rt, nil
			}),
		).
		Run()
}
