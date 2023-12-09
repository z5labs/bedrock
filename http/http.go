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
	"sync/atomic"

	"github.com/z5labs/app/http/httpvalidate"
	"github.com/z5labs/app/pkg/otelconfig"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"golang.org/x/sync/errgroup"
)

type runtimeOptions struct {
	port       uint
	mux        *http.ServeMux
	otelIniter otelconfig.Initializer
	logHandler slog.Handler
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

// TracerProvider provides an implementation for initializing a trace.TracerProvider.
func TracerProvider(initializer otelconfig.Initializer) RuntimeOption {
	return func(ro *runtimeOptions) {
		ro.otelIniter = initializer
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

// Runtime
type Runtime struct {
	port   uint
	listen func(string, string) (net.Listener, error)

	log        *slog.Logger
	otelIniter otelconfig.Initializer

	h http.Handler

	started atomic.Bool
	healthy atomic.Bool
	serving atomic.Bool
}

// NewRuntime
func NewRuntime(opts ...RuntimeOption) *Runtime {
	ros := &runtimeOptions{
		port:       8080,
		mux:        http.NewServeMux(),
		otelIniter: otelconfig.Noop,
		logHandler: noopLogHandler{},
	}
	for _, opt := range opts {
		opt(ros)
	}

	rt := &Runtime{
		port:       ros.port,
		listen:     net.Listen,
		log:        slog.New(ros.logHandler),
		otelIniter: ros.otelIniter,
		h:          ros.mux,
	}

	registerEndpoint(
		ros.mux,
		"/health/startup",
		httpvalidate.Request(
			http.HandlerFunc(rt.startupHandler),
			httpvalidate.ForMethods(http.MethodGet),
		),
	)
	registerEndpoint(
		ros.mux,
		"/health/liveness",
		httpvalidate.Request(
			http.HandlerFunc(rt.livenessHandler),
			httpvalidate.ForMethods(http.MethodGet),
		),
	)
	registerEndpoint(
		ros.mux,
		"/health/readiness",
		httpvalidate.Request(
			http.HandlerFunc(rt.readinessHandler),
			httpvalidate.ForMethods(http.MethodGet),
		),
	)

	return rt
}

// Run implements app.Runtime interface.
func (rt *Runtime) Run(ctx context.Context) error {
	ls, err := rt.listen("tcp", fmt.Sprintf(":%d", rt.port))
	if err != nil {
		return err
	}

	tp, err := rt.otelIniter.Init()
	if err != nil {
		return err
	}
	otel.SetTracerProvider(tp)

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
		defer func() {
			tp := otel.GetTracerProvider()
			stp, ok := tp.(interface {
				Shutdown(context.Context) error
			})
			if !ok {
				return
			}
			stp.Shutdown(context.Background())
		}()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		defer rt.log.Info("stopped service")

		return s.Shutdown(ctx)
	})
	g.Go(func() error {
		rt.started.Store(true)
		rt.healthy.Store(true)
		rt.serving.Store(true)
		rt.log.Info("started service")
		return s.Serve(ls)
	})

	err = g.Wait()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func registerEndpoint(mux *http.ServeMux, path string, h http.Handler) {
	mux.Handle(
		path,
		otelhttp.WithRouteTag(path, h),
	)
}

// report whether this service is ready to begin accepting traffic
func (rt *Runtime) startupHandler(w http.ResponseWriter, req *http.Request) {
	started := rt.started.Load()
	if started {
		w.WriteHeader(http.StatusOK)
		return
	}
	w.WriteHeader(http.StatusServiceUnavailable)
}

// report whether this service is healthy or needs to be restarted
func (rt *Runtime) livenessHandler(w http.ResponseWriter, req *http.Request) {
	healthy := rt.healthy.Load()
	if healthy {
		w.WriteHeader(http.StatusOK)
		return
	}
	w.WriteHeader(http.StatusServiceUnavailable)
}

// report whether this service is able to accept traffic
func (rt *Runtime) readinessHandler(w http.ResponseWriter, req *http.Request) {
	serving := rt.serving.Load()
	if serving {
		w.WriteHeader(http.StatusOK)
		return
	}
	w.WriteHeader(http.StatusServiceUnavailable)
}

type noopLogHandler struct{}

func (noopLogHandler) Enabled(_ context.Context, _ slog.Level) bool  { return true }
func (noopLogHandler) Handle(_ context.Context, _ slog.Record) error { return nil }
func (h noopLogHandler) WithAttrs(_ []slog.Attr) slog.Handler        { return h }
func (h noopLogHandler) WithGroup(name string) slog.Handler          { return h }
