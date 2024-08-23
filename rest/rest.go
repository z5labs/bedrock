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

		app.mux.Handle(
			pattern,
			otelhttp.WithRouteTag(pattern, op),
		)
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
	app.mux.HandleFunc("/openapi.json", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(w, bytes.NewReader(spec))
	})

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
