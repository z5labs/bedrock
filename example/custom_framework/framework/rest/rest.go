// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package rest

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/z5labs/bedrock/example/custom_framework/framework"
	"github.com/z5labs/bedrock/example/custom_framework/framework/internal"
	"github.com/z5labs/bedrock/example/custom_framework/framework/internal/global"

	"github.com/z5labs/bedrock"
	"github.com/z5labs/bedrock/pkg/app"
	"github.com/z5labs/bedrock/rest"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
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
				// framework.Logger will return a logger that writes to STDOUT
				// so we'll just send traces to STDERR for demo purposes.
				exp, err := stdouttrace.New(
					stdouttrace.WithWriter(os.Stderr),
				)
				if err != nil {
					return nil, err
				}

				r, err := resource.Detect(
					ctx,
					resourceDetectFunc(func(ctx context.Context) (*resource.Resource, error) {
						return resource.Default(), nil
					}),
					resource.StringDetector(semconv.SchemaURL, semconv.ServiceNameKey, func() (string, error) {
						return cfg.ServiceName, nil
					}),
					resource.StringDetector(semconv.SchemaURL, semconv.ServiceVersionKey, func() (string, error) {
						return cfg.ServiceVersion, nil
					}),
				)
				if err != nil {
					return nil, err
				}

				tp := sdktrace.NewTracerProvider(
					sdktrace.WithBatcher(exp),
					sdktrace.WithResource(r),
					sdktrace.WithSampler(sdktrace.AlwaysSample()),
				)
				ra.postRunHooks = append(ra.postRunHooks, shutdownHook(tp))

				return tp, nil
			}),
		)
	}
}

func HttpServer(cfg HttpServerConfig) Option {
	return func(ra *App) {
		ra.port = cfg.Port
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
	port         uint
	restOpts     []rest.Option
	otelOpts     []app.OTelOption
	postRunHooks []app.LifecycleHook
}

func NewApp(opts ...Option) *App {
	ra := &App{}
	for _, opt := range opts {
		opt(ra)
	}
	return ra
}

type resourceDetectFunc func(context.Context) (*resource.Resource, error)

func (f resourceDetectFunc) Detect(ctx context.Context) (*resource.Resource, error) {
	return f(ctx)
}

func (ra *App) Run(ctx context.Context) error {
	ls, err := net.Listen("tcp", fmt.Sprintf(":%d", ra.port))
	if err != nil {
		return err
	}
	ra.restOpts = append(ra.restOpts, rest.Listener(ls))

	var base bedrock.App = rest.NewApp(ra.restOpts...)

	base = app.WithLifecycleHooks(base, app.Lifecycle{
		PostRun: composePostRunHooks(ra.postRunHooks...),
	})

	base = app.WithOTel(base, ra.otelOpts...)

	base = app.WithSignalNotifications(base, os.Interrupt, os.Kill)

	return base.Run(ctx)
}

type multiErr []error

func (e multiErr) Error() string {
	var sb strings.Builder
	sb.WriteString("captured error(s):\n")
	for _, err := range e {
		sb.WriteString("\t- ")
		sb.WriteString(err.Error())
		sb.WriteByte('\n')
	}
	return sb.String()
}

func composePostRunHooks(hooks ...app.LifecycleHook) app.LifecycleHook {
	return app.LifecycleHookFunc(func(ctx context.Context) error {
		var me multiErr
		for _, hook := range hooks {
			err := hook.Run(ctx)
			if err != nil {
				me = append(me, err)
			}
		}
		if len(me) == 0 {
			return nil
		}
		return me
	})
}

type shutdown interface {
	Shutdown(context.Context) error
}

func shutdownHook(s shutdown) app.LifecycleHook {
	return app.LifecycleHookFunc(func(ctx context.Context) error {
		return s.Shutdown(ctx)
	})
}
