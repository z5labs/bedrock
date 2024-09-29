// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"slices"
	"strings"

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
func OpenApiEndpoint(method, pattern string, f func(*openapi3.Spec) http.Handler) Option {
	return func(a *App) {
		a.openApiEndpoint = func(mux *http.ServeMux) {
			mux.Handle(fmt.Sprintf("%s %s", method, pattern), f(a.spec))
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

// OpenApiJsonHandler returns an [http.Handler] which will respond with the OpenAPI schema as JSON.
func OpenApiJsonHandler(spec *openapi3.Spec) http.Handler {
	return openApiHandler{
		spec:    spec,
		marshal: json.Marshal,
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

type endpoint struct {
	method  string
	pattern string
	op      Operation
}

// Endpoint registers the [Operation] with both
// the App wide OpenAPI spec and the App wide HTTP server.
//
// "/" is always treated as "/{$}" because it would otherwise
// match too broadly and cause conflicts with other paths.
func Endpoint(method, pattern string, op Operation) Option {
	return func(app *App) {
		app.endpoints = append(app.endpoints, endpoint{
			method:  method,
			pattern: pattern,
			op:      op,
		})
	}
}

// NotFoundHandler will register the given [http.Handler] to handle
// any HTTP requests that do not match any other method-pattern combinations.
func NotFoundHandler(h http.Handler) Option {
	return func(app *App) {
		app.notFoundHandler = h
	}
}

// MethodNotAllowedHandler will register the given [http.Handler] to handle
// any HTTP requests whose method does not match the method registered to a pattern.
func MethodNotAllowedHandler(h http.Handler) Option {
	return func(app *App) {
		app.methodNotAllowedHandler = h
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

// App is a [bedrock.App] implementation to help simplify
// building RESTful applications.
type App struct {
	ls net.Listener

	spec      *openapi3.Spec
	endpoints []endpoint

	openApiEndpoint func(*http.ServeMux)

	notFoundHandler http.Handler

	pathMethods             map[string][]string
	methodNotAllowedHandler http.Handler

	listen func(network, addr string) (net.Listener, error)
}

// NewApp initializes a [App].
func NewApp(opts ...Option) *App {
	app := &App{
		spec: &openapi3.Spec{
			Openapi: "3.0.3",
		},
		pathMethods:     make(map[string][]string),
		listen:          net.Listen,
		openApiEndpoint: func(sm *http.ServeMux) {},
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

	mux := http.NewServeMux()
	app.openApiEndpoint(mux)

	err = app.registerEndpoints(mux)
	if err != nil {
		return err
	}

	if app.notFoundHandler != nil {
		mux.Handle("/{path...}", app.notFoundHandler)
	}

	app.registerMethodNotAllowedHandler(mux)

	httpServer := &http.Server{
		Handler: otelhttp.NewHandler(
			mux,
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

func (app *App) registerEndpoints(mux *http.ServeMux) error {
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
		trimmedPattern := strings.TrimSuffix(e.pattern, "{$}")
		err := app.spec.AddOperation(e.method, trimmedPattern, e.op.OpenApi())
		if err != nil {
			return err
		}

		// enforce strict matching for top-level path
		// otherwise "/" would match too broadly and http.ServeMux
		// will panic when other paths are registered e.g. /openapi.json
		if e.pattern == "/" {
			e.pattern = "/{$}"
		}
		app.pathMethods[e.pattern] = append(app.pathMethods[e.pattern], e.method)

		mux.Handle(
			fmt.Sprintf("%s %s", e.method, e.pattern),
			otelhttp.WithRouteTag(trimmedPattern, e.op),
		)
	}
	return nil
}

func (app *App) registerMethodNotAllowedHandler(mux *http.ServeMux) {
	if app.methodNotAllowedHandler == nil {
		return
	}

	// this list is pulled from the OpenAPI v3 Path Item Object documentation.
	supportedMethods := []string{
		http.MethodGet,
		http.MethodPut,
		http.MethodPost,
		http.MethodDelete,
		http.MethodOptions,
		http.MethodHead,
		http.MethodPatch,
		http.MethodTrace,
	}

	for path, methods := range app.pathMethods {
		unsupportedMethods := diffSets(supportedMethods, methods)
		for _, method := range unsupportedMethods {
			mux.Handle(fmt.Sprintf("%s %s", method, path), app.methodNotAllowedHandler)
		}
	}
}

func diffSets(xs, ys []string) []string {
	zs := make([]string, 0, len(xs))
	for _, x := range xs {
		if slices.Contains(ys, x) {
			continue
		}
		zs = append(zs, x)
	}
	return zs
}
