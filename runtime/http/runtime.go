// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package http

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/z5labs/bedrock"
	"github.com/z5labs/bedrock/config"
	"github.com/z5labs/bedrock/internal/fixedpool"
)

// BuildTCPListener creates a bedrock.Builder that constructs a TCP listener.
func BuildTCPListener(addr config.Reader[*net.TCPAddr]) bedrock.Builder[*net.TCPListener] {
	return bedrock.BuilderFunc[*net.TCPListener](func(ctx context.Context) (*net.TCPListener, error) {
		tcpAddr := config.Must(ctx, addr)
		ln, err := net.ListenTCP("tcp", tcpAddr)
		if err != nil {
			return nil, err
		}
		return ln, nil
	})
}

// BuildTLSListener creates a bedrock.Builder that wraps a base listener with TLS.
func BuildTLSListener[T net.Listener](
	base bedrock.Builder[T],
	tlsConfig config.Reader[*tls.Config],
) bedrock.Builder[net.Listener] {
	return bedrock.Map(base, func(ctx context.Context, baseListener T) (net.Listener, error) {
		return tls.NewListener(baseListener, config.Must(ctx, tlsConfig)), nil
	})
}

// Server holds the configuration for an HTTP server.
// It provides options to configure timeouts, header limits, and other server behaviors.
type Server struct {
	disableGeneralOptionsHandler config.Reader[bool]
	readTimeout                  config.Reader[time.Duration]
	readHeaderTimeout            config.Reader[time.Duration]
	writeTimeout                 config.Reader[time.Duration]
	idleTimeout                  config.Reader[time.Duration]
	maxHeaderBytes               config.Reader[int]
}

// ServerOption is a functional option for configuring a Server.
type ServerOption func(*Server)

// DisableGeneralOptionsHandler is a ServerOption that controls whether the server
// automatically replies to OPTIONS requests. When disabled, you must handle OPTIONS
// requests explicitly in your handler.
func DisableGeneralOptionsHandler(disable config.Reader[bool]) ServerOption {
	return func(srv *Server) {
		srv.disableGeneralOptionsHandler = disable
	}
}

// ReadTimeout is a ServerOption that sets the maximum duration for reading the
// entire request, including the body. The default is 5 seconds.
func ReadTimeout(d config.Reader[time.Duration]) ServerOption {
	return func(srv *Server) {
		srv.readTimeout = d
	}
}

// ReadHeaderTimeout is a ServerOption that sets the maximum duration for reading
// request headers. The default is 2 seconds.
func ReadHeaderTimeout(d config.Reader[time.Duration]) ServerOption {
	return func(srv *Server) {
		srv.readHeaderTimeout = d
	}
}

// WriteTimeout is a ServerOption that sets the maximum duration before timing out
// writes of the response. The default is 10 seconds.
func WriteTimeout(d config.Reader[time.Duration]) ServerOption {
	return func(srv *Server) {
		srv.writeTimeout = d
	}
}

// IdleTimeout is a ServerOption that sets the maximum duration to wait for the
// next request when keep-alives are enabled. The default is 120 seconds.
func IdleTimeout(d config.Reader[time.Duration]) ServerOption {
	return func(srv *Server) {
		srv.idleTimeout = d
	}
}

// MaxHeaderBytes is a ServerOption that sets the maximum number of bytes the
// server will read parsing the request header's keys and values, including the
// request line. The default is 1048576 bytes (1 MB).
func MaxHeaderBytes(n config.Reader[int]) ServerOption {
	return func(srv *Server) {
		srv.maxHeaderBytes = n
	}
}

// Runtime represents a running HTTP server application.
// It manages the lifecycle of the HTTP server and handles graceful shutdown.
type Runtime struct {
	ls  net.Listener
	srv *http.Server
}

// Run starts the HTTP server and blocks until the context is cancelled or an error occurs.
// When the context is cancelled, the server performs a graceful shutdown.
// Returns nil if the server shuts down cleanly, or an error if the server fails to start or serve.
func (r Runtime) Run(ctx context.Context) error {
	err := fixedpool.Wait(
		ctx,
		func(ctx context.Context) error {
			return r.srv.Serve(r.ls)
		},
		func(ctx context.Context) error {
			<-ctx.Done()
			return r.srv.Shutdown(context.Background())
		},
	)

	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, http.ErrServerClosed) {
		return nil
	}

	return err
}

// Build creates a bedrock.Builder that constructs an HTTP server App.
// It takes a Server configuration and an http.Handler builder, and returns a builder
// that produces a runnable App.
//
// The builder applies the Server configuration to create an http.Server with the
// provided handler. If configuration values are not set, defaults are applied as
// documented in NewServer.
func Build(listener bedrock.Builder[net.Listener], b bedrock.Builder[http.Handler], opts ...ServerOption) bedrock.Builder[Runtime] {
	return bedrock.BuilderFunc[Runtime](func(ctx context.Context) (Runtime, error) {
		ln := bedrock.MustBuild(ctx, listener)
		h := bedrock.MustBuild(ctx, b)

		srv := Server{}
		for _, opt := range opts {
			opt(&srv)
		}

		httpServer := &http.Server{
			Handler:                      h,
			DisableGeneralOptionsHandler: config.MustOr(ctx, false, srv.disableGeneralOptionsHandler),
			ReadTimeout:                  config.MustOr(ctx, 5*time.Second, srv.readTimeout),
			ReadHeaderTimeout:            config.MustOr(ctx, 2*time.Second, srv.readHeaderTimeout),
			WriteTimeout:                 config.MustOr(ctx, 10*time.Second, srv.writeTimeout),
			IdleTimeout:                  config.MustOr(ctx, 120*time.Second, srv.idleTimeout),
			MaxHeaderBytes:               config.MustOr(ctx, 1048576, srv.maxHeaderBytes),
		}

		rt := Runtime{
			ls:  ln,
			srv: httpServer,
		}

		return rt, nil
	})
}
