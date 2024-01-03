// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package grpc

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/z5labs/bedrock/pkg/health"
	"github.com/z5labs/bedrock/pkg/noop"

	"github.com/stretchr/testify/assert"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
)

type grpcServerFunc func(net.Listener) error

func (f grpcServerFunc) Serve(ls net.Listener) error {
	return f(ls)
}
func (f grpcServerFunc) GracefulStop() {}

func TestRuntime_Run(t *testing.T) {
	t.Run("will return an error", func(t *testing.T) {
		t.Run("if it fails to construct a listener", func(t *testing.T) {
			listenErr := errors.New("failed to listen")
			rt := &Runtime{
				log: slog.New(noop.LogHandler{}),
				listen: func(s1, s2 string) (net.Listener, error) {
					return nil, listenErr
				},
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			err := rt.Run(ctx)
			if !assert.Equal(t, listenErr, err) {
				return
			}
		})

		t.Run("if the grpc server fails to serve", func(t *testing.T) {
			rt := NewRuntime(ListenOnPort(0), LogHandler(noop.LogHandler{}))

			serveErr := errors.New("failed to serve")
			rt.grpc = grpcServerFunc(func(l net.Listener) error {
				return serveErr
			})

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			err := rt.Run(ctx)
			if !assert.Equal(t, serveErr, err) {
				return
			}
		})
	})

	t.Run("will not return an error", func(t *testing.T) {
		t.Run("if the grpc server is gracefully shutdown", func(t *testing.T) {
			rt := NewRuntime(
				ListenOnPort(0),
				LogHandler(noop.LogHandler{}),
			)

			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()

			err := rt.Run(ctx)
			if !assert.Nil(t, err) {
				return
			}
		})
	})
}

func TestReadiness(t *testing.T) {
	t.Run("will return serving", func(t *testing.T) {
		t.Run("if the server has just been started", func(t *testing.T) {
			rt := NewRuntime(
				LogHandler(noop.LogHandler{}),
				Service(
					func(s *grpc.Server) {},
					// No ServiceName is set so this corresponds to
					// overall server health
					Readiness(&health.Readiness{}),
				),
			)
			addrCh := make(chan net.Addr)
			rt.listen = func(s1, s2 string) (net.Listener, error) {
				defer close(addrCh)
				ls, err := net.Listen(s1, s2)
				if err != nil {
					return nil, err
				}
				addrCh <- ls.Addr()
				return ls, nil
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			g, gctx := errgroup.WithContext(ctx)
			g.Go(func() error {
				return rt.Run(gctx)
			})

			statusCh := make(chan grpc_health_v1.HealthCheckResponse_ServingStatus)
			g.Go(func() error {
				defer close(statusCh)

				addr := <-addrCh
				if addr == nil {
					return nil
				}
				conn, err := grpc.Dial(
					addr.String(),
					grpc.WithTransportCredentials(insecure.NewCredentials()),
				)
				if err != nil {
					return err
				}
				client := grpc_health_v1.NewHealthClient(conn)
				resp, err := client.Check(gctx, &grpc_health_v1.HealthCheckRequest{
					Service: "",
				})
				if err != nil {
					return err
				}
				cancel()
				statusCh <- resp.Status
				return nil
			})

			status := <-statusCh
			err := g.Wait()
			if !assert.Nil(t, err) {
				return
			}
			if !assert.Equal(t, grpc_health_v1.HealthCheckResponse_SERVING, status) {
				return
			}
		})

		t.Run("if the health metric is toggled from unhealthy to healthy", func(t *testing.T) {
			var readiness health.Readiness
			rt := NewRuntime(
				LogHandler(noop.LogHandler{}),
				Service(
					func(s *grpc.Server) {},
					// No ServiceName is set so this corresponds to
					// overall server health
					Readiness(&readiness),
				),
			)
			addrCh := make(chan net.Addr)
			rt.listen = func(s1, s2 string) (net.Listener, error) {
				defer close(addrCh)
				ls, err := net.Listen(s1, s2)
				if err != nil {
					return nil, err
				}
				addrCh <- ls.Addr()
				return ls, nil
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			g, gctx := errgroup.WithContext(ctx)
			g.Go(func() error {
				return rt.Run(gctx)
			})

			statusCh := make(chan grpc_health_v1.HealthCheckResponse_ServingStatus)
			g.Go(func() error {
				defer close(statusCh)

				addr := <-addrCh
				if addr == nil {
					return nil
				}
				conn, err := grpc.Dial(
					addr.String(),
					grpc.WithTransportCredentials(insecure.NewCredentials()),
				)
				if err != nil {
					return err
				}
				readiness.NotReady()
				readiness.Ready()

				client := grpc_health_v1.NewHealthClient(conn)
				resp, err := client.Check(gctx, &grpc_health_v1.HealthCheckRequest{
					Service: "",
				})
				if err != nil {
					return err
				}
				cancel()
				statusCh <- resp.Status
				return nil
			})

			status := <-statusCh
			err := g.Wait()
			if !assert.Nil(t, err) {
				return
			}
			if !assert.Equal(t, grpc_health_v1.HealthCheckResponse_SERVING, status) {
				return
			}
		})

		t.Run("if a specific service name is requested", func(t *testing.T) {
			rt := NewRuntime(
				LogHandler(noop.LogHandler{}),
				Service(
					func(s *grpc.Server) {},
					ServiceName("test"),
					Readiness(&health.Readiness{}),
				),
			)
			addrCh := make(chan net.Addr)
			rt.listen = func(s1, s2 string) (net.Listener, error) {
				defer close(addrCh)
				ls, err := net.Listen(s1, s2)
				if err != nil {
					return nil, err
				}
				addrCh <- ls.Addr()
				return ls, nil
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			g, gctx := errgroup.WithContext(ctx)
			g.Go(func() error {
				return rt.Run(gctx)
			})

			statusCh := make(chan grpc_health_v1.HealthCheckResponse_ServingStatus)
			g.Go(func() error {
				defer close(statusCh)

				addr := <-addrCh
				if addr == nil {
					return nil
				}
				conn, err := grpc.Dial(
					addr.String(),
					grpc.WithTransportCredentials(insecure.NewCredentials()),
				)
				if err != nil {
					return err
				}
				client := grpc_health_v1.NewHealthClient(conn)
				resp, err := client.Check(gctx, &grpc_health_v1.HealthCheckRequest{
					Service: "test",
				})
				if err != nil {
					return err
				}
				cancel()
				statusCh <- resp.Status
				return nil
			})

			status := <-statusCh
			err := g.Wait()
			if !assert.Nil(t, err) {
				return
			}
			if !assert.Equal(t, grpc_health_v1.HealthCheckResponse_SERVING, status) {
				return
			}
		})
	})

	t.Run("will return not serving", func(t *testing.T) {
		t.Run("if the health metric returns not healthy", func(t *testing.T) {
			var readiness health.Readiness
			rt := NewRuntime(
				LogHandler(noop.LogHandler{}),
				Service(
					func(s *grpc.Server) {},
					ServiceName("test"),
					Readiness(&readiness),
				),
			)
			addrCh := make(chan net.Addr)
			rt.listen = func(s1, s2 string) (net.Listener, error) {
				defer close(addrCh)
				ls, err := net.Listen(s1, s2)
				if err != nil {
					return nil, err
				}
				addrCh <- ls.Addr()
				return ls, nil
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			g, gctx := errgroup.WithContext(ctx)
			g.Go(func() error {
				return rt.Run(gctx)
			})

			statusCh := make(chan grpc_health_v1.HealthCheckResponse_ServingStatus)
			g.Go(func() error {
				defer close(statusCh)

				addr := <-addrCh
				if addr == nil {
					return nil
				}
				conn, err := grpc.Dial(
					addr.String(),
					grpc.WithTransportCredentials(insecure.NewCredentials()),
				)
				if err != nil {
					return err
				}
				readiness.NotReady()
				<-time.After(200 * time.Millisecond)

				client := grpc_health_v1.NewHealthClient(conn)
				resp, err := client.Check(gctx, &grpc_health_v1.HealthCheckRequest{
					Service: "test",
				})
				if err != nil {
					return err
				}
				cancel()
				statusCh <- resp.Status
				return nil
			})

			status := <-statusCh
			err := g.Wait()
			if !assert.Nil(t, err) {
				return
			}
			if !assert.Equal(t, grpc_health_v1.HealthCheckResponse_NOT_SERVING, status) {
				return
			}
		})
	})
}
