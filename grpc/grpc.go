// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package grpc provides a gRPC server which implements the app.Runtime interface.
package grpc

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync/atomic"

	"github.com/z5labs/app/pkg/noop"
	"github.com/z5labs/app/pkg/slogfield"

	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type runtimeOptions struct {
	port          uint
	logHandler    slog.Handler
	registerFuncs []func(*grpc.Server)
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

// Register a gRPC service with the underlying gRPC server.
func Register(f func(*grpc.Server)) RuntimeOption {
	return func(ro *runtimeOptions) {
		ro.registerFuncs = append(ro.registerFuncs, f)
	}
}

// Runtime
type Runtime struct {
	port   uint
	listen func(string, string) (net.Listener, error)

	log *slog.Logger

	started atomic.Bool
	healthy atomic.Bool
	serving atomic.Bool

	grpc *grpc.Server
}

// NewRuntime
func NewRuntime(opts ...RuntimeOption) *Runtime {
	ro := &runtimeOptions{
		port:       8090,
		logHandler: noop.LogHandler{},
	}
	for _, opt := range opts {
		opt(ro)
	}

	s := grpc.NewServer(grpc.Creds(insecure.NewCredentials()))
	for _, f := range ro.registerFuncs {
		f(s)
	}

	rt := &Runtime{
		port:   ro.port,
		listen: net.Listen,
		log:    slog.New(ro.logHandler),
		grpc:   s,
	}
	return rt
}

// Run implements the app.Runtime interface.
func (rt *Runtime) Run(ctx context.Context) error {
	ls, err := rt.listen("tcp", fmt.Sprintf(":%d", rt.port))
	if err != nil {
		rt.log.Error("failed to listen for connections", slogfield.Error(err))
		return err
	}

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		<-gctx.Done()

		rt.log.Info("shutting down service")
		rt.grpc.GracefulStop()
		rt.log.Info("shut down service")
		return nil
	})
	g.Go(func() error {
		rt.started.Store(true)
		rt.healthy.Store(true)
		rt.serving.Store(true)
		rt.log.Info("started service")
		return rt.grpc.Serve(ls)
	})

	err = g.Wait()
	if err == grpc.ErrServerStopped {
		return nil
	}
	rt.log.Error("service encountered unexpected error", slogfield.Error(err))
	return err
}
