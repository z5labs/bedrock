// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package http

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/z5labs/app/pkg/health"

	"github.com/stretchr/testify/assert"
	"golang.org/x/sync/errgroup"
)

type acceptFunc func() (net.Conn, error)

func (f acceptFunc) Accept() (net.Conn, error) {
	return f()
}

func (acceptFunc) Close() error   { return nil }
func (acceptFunc) Addr() net.Addr { return nil }

func TestRuntime_Run(t *testing.T) {
	t.Run("will return an error", func(t *testing.T) {
		t.Run("if it fails to listen", func(t *testing.T) {
			listenErr := errors.New("failed to listen")
			rt := NewRuntime(
				ListenOnPort(0),
				LogHandler(slog.Default().Handler()),
			)
			rt.listen = func(s1, s2 string) (net.Listener, error) {
				return nil, listenErr
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			err := rt.Run(ctx)
			if !assert.Equal(t, listenErr, err) {
				return
			}
		})

		t.Run("if it fails to acquire a connection", func(t *testing.T) {
			acceptErr := errors.New("failed to accept conn")
			rt := NewRuntime(
				ListenOnPort(0),
				LogHandler(slog.Default().Handler()),
			)
			rt.listen = func(s1, s2 string) (net.Listener, error) {
				ls := acceptFunc(func() (net.Conn, error) {
					return nil, acceptErr
				})
				return ls, nil
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			err := rt.Run(ctx)
			if !assert.Equal(t, acceptErr, err) {
				return
			}
		})
	})

	t.Run("will not return an error", func(t *testing.T) {
		t.Run("if the context is cancelled", func(t *testing.T) {
			rt := NewRuntime(
				ListenOnPort(0),
				LogHandler(slog.Default().Handler()),
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

func TestStarted(t *testing.T) {
	t.Run("will return 200", func(t *testing.T) {
		t.Run("if it has been started", func(t *testing.T) {
			addrCh := make(chan net.Addr)
			rt := NewRuntime(
				ListenOnPort(0),
				LogHandler(slog.Default().Handler()),
			)
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

			var statusCode int
			g, gctx := errgroup.WithContext(ctx)
			g.Go(func() error {
				return rt.Run(gctx)
			})
			g.Go(func() error {
				defer cancel()
				addr := <-addrCh
				if addr == nil {
					return nil
				}
				<-time.After(500 * time.Millisecond)

				resp, err := http.Get(fmt.Sprintf("http://%s/health/startup", addr))
				if err != nil {
					return err
				}
				defer resp.Body.Close()

				statusCode = resp.StatusCode
				return nil
			})

			err := g.Wait()
			if !assert.Nil(t, err) {
				return
			}
			if !assert.Equal(t, http.StatusOK, statusCode) {
				return
			}
		})
	})
}

func TestReadiness(t *testing.T) {
	t.Run("will return 200", func(t *testing.T) {
		t.Run("if it has just been started", func(t *testing.T) {
			var readiness health.Readiness

			addrCh := make(chan net.Addr)
			rt := NewRuntime(
				ListenOnPort(0),
				LogHandler(slog.Default().Handler()),
				Readiness(&readiness),
			)
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

			var statusCode int
			g, gctx := errgroup.WithContext(ctx)
			g.Go(func() error {
				return rt.Run(gctx)
			})
			g.Go(func() error {
				defer cancel()
				addr := <-addrCh
				if addr == nil {
					return nil
				}
				<-time.After(500 * time.Millisecond)

				resp, err := http.Get(fmt.Sprintf("http://%s/health/readiness", addr))
				if err != nil {
					return err
				}
				defer resp.Body.Close()

				statusCode = resp.StatusCode
				return nil
			})

			err := g.Wait()
			if !assert.Nil(t, err) {
				return
			}
			if !assert.Equal(t, http.StatusOK, statusCode) {
				return
			}
		})

		t.Run("if it has been marked ready", func(t *testing.T) {
			var readiness health.Readiness

			addrCh := make(chan net.Addr)
			rt := NewRuntime(
				ListenOnPort(0),
				LogHandler(slog.Default().Handler()),
				Readiness(&readiness),
			)
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

			var statusCode int
			g, gctx := errgroup.WithContext(ctx)
			g.Go(func() error {
				return rt.Run(gctx)
			})
			g.Go(func() error {
				defer cancel()
				addr := <-addrCh
				if addr == nil {
					return nil
				}
				<-time.After(500 * time.Millisecond)
				readiness.NotReady()
				readiness.Ready()

				resp, err := http.Get(fmt.Sprintf("http://%s/health/readiness", addr))
				if err != nil {
					return err
				}
				defer resp.Body.Close()

				statusCode = resp.StatusCode
				return nil
			})

			err := g.Wait()
			if !assert.Nil(t, err) {
				return
			}
			if !assert.Equal(t, http.StatusOK, statusCode) {
				return
			}
		})
	})

	t.Run("will return 503", func(t *testing.T) {
		t.Run("if it has been marked not ready", func(t *testing.T) {
			var readiness health.Readiness

			addrCh := make(chan net.Addr)
			rt := NewRuntime(
				ListenOnPort(0),
				LogHandler(slog.Default().Handler()),
				Readiness(&readiness),
			)
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

			var statusCode int
			g, gctx := errgroup.WithContext(ctx)
			g.Go(func() error {
				return rt.Run(gctx)
			})
			g.Go(func() error {
				defer cancel()
				addr := <-addrCh
				if addr == nil {
					return nil
				}
				<-time.After(500 * time.Millisecond)
				readiness.NotReady()

				resp, err := http.Get(fmt.Sprintf("http://%s/health/readiness", addr))
				if err != nil {
					return err
				}
				defer resp.Body.Close()

				statusCode = resp.StatusCode
				return nil
			})

			err := g.Wait()
			if !assert.Nil(t, err) {
				return
			}
			if !assert.Equal(t, http.StatusServiceUnavailable, statusCode) {
				return
			}
		})
	})
}

func TestLiveness(t *testing.T) {
	t.Run("will return 200", func(t *testing.T) {
		t.Run("if it has just been started", func(t *testing.T) {
			var liveness health.Liveness

			addrCh := make(chan net.Addr)
			rt := NewRuntime(
				ListenOnPort(0),
				LogHandler(slog.Default().Handler()),
				Liveness(&liveness),
			)
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

			var statusCode int
			g, gctx := errgroup.WithContext(ctx)
			g.Go(func() error {
				return rt.Run(gctx)
			})
			g.Go(func() error {
				defer cancel()
				addr := <-addrCh
				if addr == nil {
					return nil
				}
				<-time.After(500 * time.Millisecond)

				resp, err := http.Get(fmt.Sprintf("http://%s/health/liveness", addr))
				if err != nil {
					return err
				}
				defer resp.Body.Close()

				statusCode = resp.StatusCode
				return nil
			})

			err := g.Wait()
			if !assert.Nil(t, err) {
				return
			}
			if !assert.Equal(t, http.StatusOK, statusCode) {
				return
			}
		})

		t.Run("if it has been marked alive", func(t *testing.T) {
			var liveness health.Liveness

			addrCh := make(chan net.Addr)
			rt := NewRuntime(
				ListenOnPort(0),
				LogHandler(slog.Default().Handler()),
				Liveness(&liveness),
			)
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

			var statusCode int
			g, gctx := errgroup.WithContext(ctx)
			g.Go(func() error {
				return rt.Run(gctx)
			})
			g.Go(func() error {
				defer cancel()
				addr := <-addrCh
				if addr == nil {
					return nil
				}
				<-time.After(500 * time.Millisecond)
				liveness.Dead()
				liveness.Alive()

				resp, err := http.Get(fmt.Sprintf("http://%s/health/liveness", addr))
				if err != nil {
					return err
				}
				defer resp.Body.Close()

				statusCode = resp.StatusCode
				return nil
			})

			err := g.Wait()
			if !assert.Nil(t, err) {
				return
			}
			if !assert.Equal(t, http.StatusOK, statusCode) {
				return
			}
		})
	})

	t.Run("will return 503", func(t *testing.T) {
		t.Run("if it has been marked dead", func(t *testing.T) {
			var liveness health.Liveness

			addrCh := make(chan net.Addr)
			rt := NewRuntime(
				ListenOnPort(0),
				LogHandler(slog.Default().Handler()),
				Liveness(&liveness),
			)
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

			var statusCode int
			g, gctx := errgroup.WithContext(ctx)
			g.Go(func() error {
				return rt.Run(gctx)
			})
			g.Go(func() error {
				defer cancel()
				addr := <-addrCh
				if addr == nil {
					return nil
				}
				<-time.After(500 * time.Millisecond)
				liveness.Dead()

				resp, err := http.Get(fmt.Sprintf("http://%s/health/liveness", addr))
				if err != nil {
					return err
				}
				defer resp.Body.Close()

				statusCode = resp.StatusCode
				return nil
			})

			err := g.Wait()
			if !assert.Nil(t, err) {
				return
			}
			if !assert.Equal(t, http.StatusServiceUnavailable, statusCode) {
				return
			}
		})
	})
}
