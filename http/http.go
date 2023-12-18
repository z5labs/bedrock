// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package http provides a HTTP server which implements the app.Runtime interface.
package http

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/http"

	"github.com/z5labs/bedrock/http/httpvalidate"
	"github.com/z5labs/bedrock/pkg/health"
	"github.com/z5labs/bedrock/pkg/noop"
	"github.com/z5labs/bedrock/pkg/slogfield"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"golang.org/x/sync/errgroup"
)

type runtimeOptions struct {
	port       uint
	mux        *http.ServeMux
	logHandler slog.Handler
	readiness  *health.Readiness
	liveness   *health.Liveness
	tlsConfig  *tls.Config
	http2Only  bool
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

// TLSConfig
func TLSConfig(cfg *tls.Config) RuntimeOption {
	return func(ro *runtimeOptions) {
		ro.tlsConfig = cfg
	}
}

// Http2Only
func Http2Only() RuntimeOption {
	return func(ro *runtimeOptions) {
		ro.http2Only = true
	}
}

// Runtime
type Runtime struct {
	port   uint
	listen func(string, string) (net.Listener, error)

	log *slog.Logger

	tlsConfig *tls.Config
	http2Only bool
	h         http.Handler

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
		tlsConfig: ros.tlsConfig,
		http2Only: ros.http2Only,
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
	if rt.tlsConfig != nil {
		rt.tlsConfig.NextProtos = append([]string{"h2"}, rt.tlsConfig.NextProtos...)
		if rt.http2Only {
			rt.tlsConfig.NextProtos = []string{"h2"}
		}
		ls = tls.NewListener(ls, rt.tlsConfig)
	}

	s := &http.Server{
		Handler: otelhttp.NewHandler(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if rt.http2Only && r.ProtoMajor < 2 {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				rt.h.ServeHTTP(w, r)
			}),
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
	if err == nil || err == http.ErrServerClosed {
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
