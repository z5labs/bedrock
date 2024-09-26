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

	"github.com/swaggest/openapi-go/openapi3"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"golang.org/x/sync/errgroup"
)

// Option represents configurable attributes of [App].
type Option func(*App)

// ListenOn allows you to configure the port which
// the underlying HTTP server will listen for incoming
// connections.
//
// Default: 80
func ListenOn(port uint) Option {
	return func(a *App) {
		a.port = port
	}
}

// Operation represents anything that can handle HTTP requests
// and provide OpenAPI documentation for itself.
type Operation interface {
	http.Handler

	OpenApi() openapi3.Operation
}

// Endpoint registers the [Operation] with both
// the App wide OpenAPI spec and the App wide HTTP server.
func Endpoint(method, pattern string, op Operation) Option {
	return func(app *App) {
		err := app.spec.AddOperation(method, pattern, op.OpenApi())
		if err != nil {
			panic(err)
		}
		app.pathMethods[pattern] = append(app.pathMethods[pattern], method)

		app.mux.Handle(
			pattern,
			otelhttp.WithRouteTag(pattern, op),
		)
	}
}

// NotFoundHandler will register the given [http.Handler] to handle
// any HTTP requests that do not match any other method-pattern combinations.
func NotFoundHandler(h http.Handler) Option {
	return func(app *App) {
		app.mux.Handle("/{path...}", h)
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
	port uint
	spec *openapi3.Spec
	mux  *http.ServeMux

	pathMethods             map[string][]string
	methodNotAllowedHandler http.Handler

	listen      func(network, addr string) (net.Listener, error)
	marshalJSON func(any) ([]byte, error)
}

// NewApp initializes a [App].
func NewApp(opts ...Option) *App {
	app := &App{
		port: 80,
		spec: &openapi3.Spec{
			Openapi: "3.0.3",
		},
		mux:         http.NewServeMux(),
		pathMethods: make(map[string][]string),
		listen:      net.Listen,
		marshalJSON: json.Marshal,
	}
	for _, opt := range opts {
		opt(app)
	}
	return app
}

// Run implements the [bedrock.App] interface.
func (app *App) Run(ctx context.Context) error {
	spec, err := app.marshalJSON(app.spec)
	if err != nil {
		return err
	}
	app.mux.HandleFunc(
		fmt.Sprintf("%s /openapi.json", http.MethodGet),
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.Copy(w, bytes.NewReader(spec))
		},
	)

	app.registerMethodNotAllowedHandler()

	ls, err := app.listen("tcp", fmt.Sprintf(":%d", app.port))
	if err != nil {
		return err
	}
	defer func() {
		_ = ls.Close()
	}()

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

func (app *App) registerMethodNotAllowedHandler() {
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
			app.mux.Handle(fmt.Sprintf("%s %s", method, path), app.methodNotAllowedHandler)
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
