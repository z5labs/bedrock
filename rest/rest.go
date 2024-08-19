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

	"github.com/z5labs/bedrock/rest/endpoint"

	"github.com/swaggest/openapi-go/openapi3"
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

// Endpoint
func Endpoint[Req, Resp any](e *endpoint.Endpoint[Req, Resp]) Option {
	return func(app *App) {
		e.OpenApi(app.spec)

		app.mux.Handle(e.Pattern(), e)
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
