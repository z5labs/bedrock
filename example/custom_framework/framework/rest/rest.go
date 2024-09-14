// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package rest

import (
	"bytes"
	"context"
	_ "embed"
	"net/http"
	"os"

	"github.com/z5labs/bedrock/example/custom_framework/framework"
	"github.com/z5labs/bedrock/example/custom_framework/framework/internal"
	"github.com/z5labs/bedrock/example/custom_framework/framework/internal/global"

	"github.com/z5labs/bedrock"
	"github.com/z5labs/bedrock/pkg/app"
	"github.com/z5labs/bedrock/rest"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

//go:embed default_config.yaml
var configBytes []byte

func init() {
	global.RegisterConfigSource(internal.ConfigSource(bytes.NewReader(configBytes)))
}

type OpenApiConfig struct {
	Title   string `config:"title"`
	Version string `config:"version"`
}

type HttpServerConfig struct {
	Port uint `config:"port"`
}

type Config struct {
	framework.Config `config:",squash"`

	OpenApi OpenApiConfig `config:"openapi"`

	Http HttpServerConfig `config:"http"`
}

type Option func(*App)

func OpenApi(cfg OpenApiConfig) Option {
	return func(ra *App) {
		ra.restOpts = append(
			ra.restOpts,
			rest.Title(cfg.Title),
			rest.Version(cfg.Version),
		)
	}
}

func OTel(cfg framework.OTelConfig) Option {
	return func(ra *App) {
		ra.otelOpts = append(
			ra.otelOpts,
			app.OTelTextMapPropogator(func(ctx context.Context) (propagation.TextMapPropagator, error) {
				tmp := propagation.NewCompositeTextMapPropagator(propagation.Baggage{}, propagation.TraceContext{})
				return tmp, nil
			}),
			app.OTelTracerProvider(func(ctx context.Context) (trace.TracerProvider, error) {
				return nil, nil
			}),
		)
	}
}

func HttpServer(cfg HttpServerConfig) Option {
	return func(ra *App) {
		ra.restOpts = append(ra.restOpts, rest.ListenOn(cfg.Port))
	}
}

type Endpoint struct {
	Method    string
	Path      string
	Operation Operation
}

// ServeHTTP implements the http.Handler interface by simply just calling
// ServeHTTP on the Operation for the Endpoint. This method is only implemented
// as a convenience to simplify unit testing.
func (e Endpoint) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	e.Operation.ServeHTTP(w, r)
}

func WithEndpoint(e Endpoint) Option {
	return func(ra *App) {
		ra.restOpts = append(ra.restOpts, rest.Endpoint(e.Method, e.Path, e.Operation))
	}
}

type App struct {
	restOpts []rest.Option
	otelOpts []app.OTelOption
}

func NewApp(opts ...Option) *App {
	ra := &App{}
	for _, opt := range opts {
		opt(ra)
	}
	return ra
}

func (ra *App) Run(ctx context.Context) error {
	var base bedrock.App = rest.NewApp(ra.restOpts...)

	base = app.WithOTel(
		base,
		app.OTelTracerProvider(func(ctx context.Context) (trace.TracerProvider, error) {
			return nil, nil
		}),
	)

	base = app.WithSignalNotifications(base, os.Interrupt, os.Kill)

	return base.Run(ctx)
}
