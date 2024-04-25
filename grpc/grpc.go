// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package grpc

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/z5labs/bedrock/pkg/health"
	"github.com/z5labs/bedrock/pkg/noop"
	"github.com/z5labs/bedrock/pkg/slogfield"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	grpchealth "google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

type service struct {
	registerFunc func(*grpc.Server)
	opts         serviceOptions
}

type runtimeOptions struct {
	port          uint
	logHandler    slog.Handler
	tc            credentials.TransportCredentials
	services      []service
	serverOptions []grpc.ServerOption
}

// RuntimeOption are options for configuring the gRPC runtime.
type RuntimeOption func(*runtimeOptions)

// ListenOnPort will configure the HTTP server to listen on the given port.
//
// Default port is 8080.
func ListenOnPort(port uint) RuntimeOption {
	return func(ro *runtimeOptions) {
		ro.port = port
	}
}

// LogHandler configures the underlying slog.Handler.
func LogHandler(h slog.Handler) RuntimeOption {
	return func(ro *runtimeOptions) {
		ro.logHandler = h
	}
}

// TransportCredentials configures the gRPC transport credentials which the gRPC server uses.
func TransportCredentials(tc credentials.TransportCredentials) RuntimeOption {
	return func(ro *runtimeOptions) {
		ro.tc = tc
	}
}

// UnaryInterceptor configures the gRPC server for one unary interceptors, please refer to grpc.UnaryInterceptor for more information
func UnaryInterceptor(f grpc.UnaryServerInterceptor) RuntimeOption {
	return func(ro *runtimeOptions) {
		ro.serverOptions = append(ro.serverOptions, grpc.UnaryInterceptor(f))
	}
}

// ChainUnaryInterceptor configures the gRPC server for multiple unary interceptors, please refer to grpc.ChainUnaryInterceptor for more information
func ChainUnaryInterceptor(interceptors ...grpc.UnaryServerInterceptor) RuntimeOption {
	return func(ro *runtimeOptions) {
		ro.serverOptions = append(ro.serverOptions, grpc.ChainUnaryInterceptor(interceptors...))
	}
}

// StreamInterceptor configures the gRPC server for one server interceptor, please refer to grpc.StreamInterceptor for more information
func StreamInterceptor(i grpc.StreamServerInterceptor) RuntimeOption {
	return func(ro *runtimeOptions) {
		ro.serverOptions = append(ro.serverOptions, grpc.StreamInterceptor(i))
	}
}

// ChainStreamInterceptor configires the gRPC server for multiple server interceptors, please refer to grpc.ChainStreamInterceptor for more information
func ChainStreamInterceptor(interceptors ...grpc.StreamServerInterceptor) RuntimeOption {
	return func(ro *runtimeOptions) {
		ro.serverOptions = append(ro.serverOptions, grpc.ChainStreamInterceptor(interceptors...))
	}
}

type serviceOptions struct {
	name      string
	readiness health.Metric
}

// ServiceOption are options for configuring the gRPC health service.
type ServiceOption func(*serviceOptions)

// ServiceName configures the service name which will be reported by the gRPC health service.
func ServiceName(name string) ServiceOption {
	return func(so *serviceOptions) {
		so.name = name
	}
}

// Readiness configures the health readiness metric for the gRPC service.
func Readiness(m health.Metric) ServiceOption {
	return func(so *serviceOptions) {
		so.readiness = m
	}
}

// Service registers a gRPC service with the underlying gRPC server.
func Service(f func(*grpc.Server), opts ...ServiceOption) RuntimeOption {
	return func(ro *runtimeOptions) {
		so := serviceOptions{
			readiness: &health.Binary{},
		}
		for _, opt := range opts {
			opt(&so)
		}
		ro.services = append(ro.services, service{
			registerFunc: f,
			opts:         so,
		})
	}
}

type serviceHealthMonitor struct {
	name      string
	readiness health.Metric
}

type grpcServer interface {
	Serve(net.Listener) error
	GracefulStop()
}

// Runtime is a bedrock.Runtime for running a gRPC service.
type Runtime struct {
	port   uint
	listen func(string, string) (net.Listener, error)

	log *slog.Logger

	serviceHealthMonitors []serviceHealthMonitor

	grpc   grpcServer
	health *grpchealth.Server
}

// NewRuntime returns a fully initialized gRPC Runtime.
func NewRuntime(opts ...RuntimeOption) *Runtime {
	ro := &runtimeOptions{
		port:          8090,
		logHandler:    noop.LogHandler{},
		tc:            insecure.NewCredentials(),
		serverOptions: []grpc.ServerOption{},
	}
	for _, opt := range opts {
		opt(ro)
	}

	var healthMonitors []serviceHealthMonitor
	s := grpc.NewServer(
		append(ro.serverOptions,
			[]grpc.ServerOption{
				grpc.StatsHandler(otelgrpc.NewServerHandler(
					otelgrpc.WithMessageEvents(otelgrpc.ReceivedEvents, otelgrpc.SentEvents),
				)),
				grpc.Creds(ro.tc),
			}...,
		)...,
	)
	for _, svc := range ro.services {
		svc.registerFunc(s)
		healthMonitors = append(healthMonitors, serviceHealthMonitor{
			name:      svc.opts.name,
			readiness: svc.opts.readiness,
		})
	}

	healthServer := grpchealth.NewServer()
	grpc_health_v1.RegisterHealthServer(s, healthServer)

	rt := &Runtime{
		port:                  ro.port,
		listen:                net.Listen,
		log:                   slog.New(ro.logHandler),
		serviceHealthMonitors: healthMonitors,
		grpc:                  s,
		health:                healthServer,
	}
	return rt
}

// Run implements the app.Runtime interface.
func (rt *Runtime) Run(ctx context.Context) error {
	ls, err := rt.listen("tcp", fmt.Sprintf(":%d", rt.port))
	if err != nil {
		rt.log.ErrorContext(ctx, "failed to listen for connections", slogfield.Error(err))
		return err
	}

	g, gctx := errgroup.WithContext(ctx)
	for _, monitor := range rt.serviceHealthMonitors {
		monitor := monitor
		g.Go(func() error {
			healthy := true
			rt.health.SetServingStatus(monitor.name, grpc_health_v1.HealthCheckResponse_SERVING)
			for {
				select {
				case <-gctx.Done():
					return nil
				case <-time.After(200 * time.Millisecond):
				}

				isHealthy := monitor.readiness.Healthy(gctx)
				if healthy && isHealthy {
					continue
				}
				healthy = isHealthy
				rt.health.SetServingStatus(monitor.name, getServingStatus(isHealthy))
			}
		})
	}
	g.Go(func() error {
		<-gctx.Done()

		rt.log.InfoContext(gctx, "shutting down service")
		rt.grpc.GracefulStop()
		rt.log.InfoContext(gctx, "shut down service")
		return nil
	})
	g.Go(func() error {
		rt.log.InfoContext(gctx, "started service")
		return rt.grpc.Serve(ls)
	})

	err = g.Wait()
	if err == nil || err == grpc.ErrServerStopped {
		return nil
	}
	rt.log.ErrorContext(gctx, "service encountered unexpected error", slogfield.Error(err))
	return err
}

func getServingStatus(healthy bool) grpc_health_v1.HealthCheckResponse_ServingStatus {
	if healthy {
		return grpc_health_v1.HealthCheckResponse_SERVING
	}
	return grpc_health_v1.HealthCheckResponse_NOT_SERVING
}
