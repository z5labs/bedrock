// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package rest

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/z5labs/bedrock/rest/endpoint"
	"github.com/z5labs/bedrock/rest/mux"

	"github.com/swaggest/openapi-go/openapi3"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"golang.org/x/sync/errgroup"
	"gopkg.in/yaml.v3"
)

// Option represents configurable attributes of [App].
type Option func(*App)

// Listener allows you to configure the [net.Listener] for
// the underlying [http.Server] to use for serving requests.
//
// If this option is not supplied, then [net.Listen] will be
// used to create a [net.Listener] for "tcp" and address ":80".
func Listener(ls net.Listener) Option {
	return func(a *App) {
		a.ls = ls
	}
}

// OpenApiEndpoint registers a [http.Handler] with the underlying [http.ServeMux]
// meant for serving the OpenAPI schema.
func OpenApiEndpoint(method mux.Method, pattern string, f func(*openapi3.Spec) http.Handler) Option {
	return func(a *App) {
		a.openApiEndpoint = func(mux Mux) {
			mux.Handle(method, pattern, f(a.spec))
		}
	}
}

type openApiHandler struct {
	spec    *openapi3.Spec
	marshal func(any) ([]byte, error)
}

func (h openApiHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b, err := h.marshal(h.spec)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	_, _ = io.Copy(w, bytes.NewReader(b))
}

type specHandler struct {
	spec *openapi3.Spec
}

func (h *specHandler) Handle(ctx context.Context) (*openapi3.Spec, error) {
	return h.spec, nil
}

// OpenApiJsonHandler returns an [http.Handler] which will respond with the OpenAPI schema as JSON.
func OpenApiJsonHandler(eh endpoint.ErrorHandler) func(*openapi3.Spec) http.Handler {
	return func(spec *openapi3.Spec) http.Handler {
		h := &specHandler{
			spec: spec,
		}

		return endpoint.NewOperation(
			endpoint.ProducesJson(
				endpoint.ConsumesNothing(
					h,
				),
			),
			endpoint.OnError(eh),
		)
	}
}

// OpenApiYamlHandler returns an [http.Handler] which will respond with the OpenAPI schema as YAML.
func OpenApiYamlHandler(spec *openapi3.Spec) http.Handler {
	return openApiHandler{
		spec:    spec,
		marshal: yaml.Marshal,
	}
}

// Operation represents anything that can handle HTTP requests
// and provide OpenAPI documentation for itself.
type Operation interface {
	http.Handler

	OpenApi() openapi3.Operation
}

// Endpoint represents all information necessary for registering
// an [Operation] with a [App].
type Endpoint struct {
	Method    mux.Method
	Pattern   string
	Operation Operation
}

// Register registers the [Endpoint] with both
// the App wide OpenAPI spec and the App wide HTTP server.
//
// "/" is always treated as "/{$}" because it would otherwise
// match too broadly and cause conflicts with other paths.
func Register(e Endpoint) Option {
	return func(app *App) {
		app.endpoints = append(app.endpoints, e)
	}
}

// Title sets the title of the API in its OpenAPI spec.
//
// In order for your OpenAPI spec to be fully compliant
// with other tooling, this option is required.
func Title(s string) Option {
	return func(a *App) {
		a.spec.Info.Title = s
	}
}

// Version sets the API version in its OpenAPI spec.
//
// In order for your OpenAPI spec to be fully compliant
// with other tooling, this option is required.
func Version(s string) Option {
	return func(a *App) {
		a.spec.Info.Version = s
	}
}

// Mux
type Mux interface {
	http.Handler

	Handle(method mux.Method, pattern string, h http.Handler)
}

// WithMux
func WithMux(m Mux) Option {
	return func(a *App) {
		a.mux = m
	}
}

// App is a [bedrock.App] implementation to help simplify
// building RESTful applications.
type App struct {
	ls net.Listener

	spec      *openapi3.Spec
	mux       Mux
	endpoints []Endpoint

	openApiEndpoint func(Mux)

	listen func(network, addr string) (net.Listener, error)
}

// NewApp initializes a [App].
func NewApp(opts ...Option) *App {
	app := &App{
		spec: &openapi3.Spec{
			Openapi: "3.0.3",
		},
		mux:             mux.NewHttp(),
		listen:          net.Listen,
		openApiEndpoint: func(_ Mux) {},
	}
	for _, opt := range opts {
		opt(app)
	}
	return app
}

// Run implements the [bedrock.App] interface.
func (app *App) Run(ctx context.Context) error {
	ls, err := app.listener()
	if err != nil {
		return err
	}

	app.openApiEndpoint(app.mux)

	err = app.registerEndpoints()
	if err != nil {
		return err
	}

	httpServer := &http.Server{
		Handler: otelhttp.NewHandler(
			app.mux,
			"server",
			otelhttp.WithMessageEvents(otelhttp.ReadEvents, otelhttp.WriteEvents),
		),
	}

	eg, egctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		return httpServer.Serve(ls)
	})
	eg.Go(func() error {
		<-egctx.Done()
		return httpServer.Shutdown(context.Background())
	})

	err = eg.Wait()
	if err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (app *App) listener() (net.Listener, error) {
	if app.ls != nil {
		return app.ls, nil
	}
	return app.listen("tcp", ":80")
}

func (app *App) registerEndpoints() error {
	for _, e := range app.endpoints {
		// Per the net/http.ServeMux docs, https://pkg.go.dev/net/http#ServeMux:
		//
		// 		The special wildcard {$} matches only the end of the URL.
		//      For example, the pattern "/{$}" matches only the path "/",
		//      whereas the pattern "/" matches every path.
		//
		// This means that when registering the pattern with the OpenAPI spec
		// the {$} needs to be stripped because OpenAPI will believe it's
		// an actual path parameter.
		trimmedPattern := strings.TrimSuffix(e.Pattern, "{$}")

		// Per the net/http.ServeMux docs, https://pkg.go.dev/net/http#ServeMux:
		//
		//      A path can include wildcard segments of the form {NAME} or {NAME...}.
		//
		// The '...' wildcard has no equivalent in OpenAPI so we must remove it
		// before registering the OpenAPI operation with the spec.
		trimmedPattern = strings.ReplaceAll(trimmedPattern, "...", "")

		err := app.spec.AddOperation(string(e.Method), trimmedPattern, e.Operation.OpenApi())
		if err != nil {
			return err
		}

		// enforce strict matching for top-level path
		// otherwise "/" would match too broadly and http.ServeMux
		// will panic when other paths are registered e.g. /openapi.json
		if e.Pattern == "/" {
			e.Pattern = "/{$}"
		}

		app.mux.Handle(
			e.Method,
			e.Pattern,
			otelhttp.WithRouteTag(trimmedPattern, e.Operation),
		)
	}
	return nil
}
