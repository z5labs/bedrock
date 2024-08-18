// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package rest

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"

	"github.com/swaggest/openapi-go/openapi3"
	"github.com/z5labs/bedrock/rest/endpoint"
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

// Empty
type Empty struct{}

// Endpoint
func Endpoint[Req, Resp any](e *endpoint.Endpoint[Req, Resp]) Option {
	return func(app *App) {
		oc, err := app.openapi.NewOperationContext(e.Method(), e.Pattern())
		if err != nil {
			panic(err)
		}
		defer app.openapi.AddOperation(oc)

		e.OpenApi(oc)

		app.mux.Handle(e.Pattern(), e)
	}
}

// App is a [bedrock.App] implementation to help simplify
// building RESTful applications.
type App struct {
	port    uint
	openapi *openapi3.Reflector
	mux     *http.ServeMux

	listen func(network, addr string) (net.Listener, error)
}

// NewApp initializes a [App].
func NewApp(opts ...Option) *App {
	app := &App{
		port: 80,
		openapi: &openapi3.Reflector{
			Spec: &openapi3.Spec{
				Openapi: "3.0.3",
			},
		},
		mux:    http.NewServeMux(),
		listen: net.Listen,
	}
	for _, opt := range opts {
		opt(app)
	}
	return app
}

// Run implements the [bedrock.App] interface.
func (app *App) Run(ctx context.Context) error {
	spec, err := app.openapi.Spec.MarshalJSON()
	if err != nil {
		return err
	}
	app.mux.HandleFunc("/openapi.json", func(w http.ResponseWriter, r *http.Request) {
		io.Copy(w, bytes.NewReader(spec))
	})

	ls, err := app.listen("tcp", fmt.Sprintf(":%d", app.port))
	if err != nil {
		return err
	}
	defer func() {
		_ = ls.Close()
	}()

	httpServer := &http.Server{
		Handler: app.mux,
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
