// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package http provides a HTTP server which implements the app.Runtime interface.
package http

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"

	"github.com/z5labs/app/http/httpvalidate"
	"github.com/z5labs/app/pkg/health"
	"github.com/z5labs/app/pkg/noop"
	"github.com/z5labs/app/pkg/slogfield"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"golang.org/x/sync/errgroup"
)

type runtimeOptions struct {
	port       uint
	mux        *http.ServeMux
	logHandler slog.Handler
	readiness  *health.Readiness
	liveness   *health.Liveness
}

// RuntimeOption
type RuntimeOption func(*runtimeOptions)

// ListenOnPort will configure the HTTP server to listen on the given port.
//
// Default port is 8080.
func ListenOnPort(port uint) RuntimeOption {
	return func(ro *runtimeOptions) {
		ro.port = port
	}
}

// LogHandler
func LogHandler(h slog.Handler) RuntimeOption {
	return func(ro *runtimeOptions) {
		ro.logHandler = h
	}
}

// Handle registers a http.Handler for the given path pattern.
func Handle(pattern string, h http.Handler) RuntimeOption {
	return func(ro *runtimeOptions) {
		registerEndpoint(ro.mux, pattern, h)
	}
}

// HandleFunc registers a http.HandlerFunc for the given path pattern.
func HandleFunc(pattern string, f func(http.ResponseWriter, *http.Request)) RuntimeOption {
	return func(ro *runtimeOptions) {
		registerEndpoint(ro.mux, pattern, http.HandlerFunc(f))
	}
}

// Readiness
func Readiness(r *health.Readiness) RuntimeOption {
	return func(ro *runtimeOptions) {
		ro.readiness = r
	}
}

// Liveness
func Liveness(l *health.Liveness) RuntimeOption {
	return func(ro *runtimeOptions) {
		ro.liveness = l
	}
}

// Runtime
type Runtime struct {
	port   uint
	listen func(string, string) (net.Listener, error)

	log *slog.Logger

	h http.Handler

	started   *health.Started
	liveness  *health.Liveness
	readiness *health.Readiness
}

// NewRuntime
func NewRuntime(opts ...RuntimeOption) *Runtime {
	ros := &runtimeOptions{
		port:       8080,
		mux:        http.NewServeMux(),
		logHandler: noop.LogHandler{},
		readiness:  &health.Readiness{},
		liveness:   &health.Liveness{},
	}
	for _, opt := range opts {
		opt(ros)
	}

	rt := &Runtime{
		port:      ros.port,
		listen:    net.Listen,
		log:       slog.New(ros.logHandler),
		h:         ros.mux,
		started:   &health.Started{},
		liveness:  ros.liveness,
		readiness: ros.readiness,
	}

	registerEndpoint(
		ros.mux,
		"/health/startup",
		httpvalidate.Request(
			rt.started,
			httpvalidate.ForMethods(http.MethodGet),
		),
	)
	registerEndpoint(
		ros.mux,
		"/health/liveness",
		httpvalidate.Request(
			rt.liveness,
			httpvalidate.ForMethods(http.MethodGet),
		),
	)
	registerEndpoint(
		ros.mux,
		"/health/readiness",
		httpvalidate.Request(
			rt.readiness,
			httpvalidate.ForMethods(http.MethodGet),
		),
	)

	return rt
}

// Run implements app.Runtime interface.
func (rt *Runtime) Run(ctx context.Context) error {
	ls, err := rt.listen("tcp", fmt.Sprintf(":%d", rt.port))
	if err != nil {
		rt.log.Error("failed to listen for connections", slogfield.Error(err))
		return err
	}

	s := &http.Server{
		Handler: otelhttp.NewHandler(
			rt.h,
			"server",
			otelhttp.WithMessageEvents(otelhttp.ReadEvents, otelhttp.WriteEvents),
		),
	}

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		<-gctx.Done()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		defer rt.log.Info("shut down service")

		rt.log.Info("shutting down service")
		return s.Shutdown(ctx)
	})
	g.Go(func() error {
		rt.started.Started()
		rt.liveness.Alive()
		rt.readiness.Ready()
		rt.log.Info("started service")
		return s.Serve(ls)
	})

	err = g.Wait()
	if err == http.ErrServerClosed {
		return nil
	}
	rt.log.Error("service encountered unexpected error", slogfield.Error(err))
	return err
}

func registerEndpoint(mux *http.ServeMux, path string, h http.Handler) {
	mux.Handle(
		path,
		otelhttp.WithRouteTag(path, h),
	)
}
