// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package rest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/swaggest/openapi-go/openapi3"
	"golang.org/x/sync/errgroup"
)

func TestNotFoundHandler(t *testing.T) {
	testCases := []struct {
		Name            string
		RegisterPattern string
		RequestPath     string
		NotFound        bool
	}{
		{
			Name:        "should match not found if no other endpoints are registered and '/' is requested",
			RequestPath: "/",
			NotFound:    true,
		},
		{
			Name:        "should match not found if no other endpoints are registered and a sub path is requested",
			RequestPath: "/hello",
			NotFound:    true,
		},
		{
			Name:            "should match not found if other endpoints are registered and '/' is requested",
			RegisterPattern: "/hello",
			RequestPath:     "/",
			NotFound:        true,
		},
		{
			Name:            "should match not found if other endpoints are registered and unknown sub-path is requested",
			RegisterPattern: "/hello",
			RequestPath:     "/bye",
			NotFound:        true,
		},
		{
			Name:            "should match not found if '/{$}' is registered and a sub-path is requested",
			RegisterPattern: "/{$}",
			RequestPath:     "/bye",
			NotFound:        true,
		},
		{
			Name:            "should not match not found if endpoint pattern is requested",
			RegisterPattern: "/hello",
			RequestPath:     "/hello",
			NotFound:        false,
		},
		{
			Name:            "should not match not found if '/{$}' is registered and '/' requested",
			RegisterPattern: "/{$}",
			RequestPath:     "/",
			NotFound:        false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.Name, func(t *testing.T) {
			notFoundHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)

				enc := json.NewEncoder(w)
				enc.Encode(map[string]any{"hello": "world"})
			})

			addrCh := make(chan net.Addr)
			app := NewApp(
				func(a *App) {
					a.listen = func(network, addr string) (net.Listener, error) {
						ls, err := net.Listen(network, ":0")
						if err != nil {
							return nil, err
						}
						defer close(addrCh)

						addrCh <- ls.Addr()
						return ls, nil
					}
				},
				NotFoundHandler(notFoundHandler),
			)

			if testCase.RegisterPattern != "" {
				app.mux.HandleFunc(testCase.RegisterPattern, func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
				})
			}

			respCh := make(chan *http.Response, 1)
			ctx, cancel := context.WithCancel(context.Background())
			eg, egctx := errgroup.WithContext(ctx)
			eg.Go(func() error {
				return app.Run(egctx)
			})
			eg.Go(func() error {
				defer cancel()
				defer close(respCh)

				addr := <-addrCh
				resp, err := http.Get(fmt.Sprintf("http://%s/%s", addr, testCase.RequestPath))
				if err != nil {
					return err
				}

				select {
				case <-egctx.Done():
					return egctx.Err()
				case respCh <- resp:
				}
				return nil
			})

			err := eg.Wait()
			if !assert.Nil(t, err) {
				return
			}

			resp := <-respCh
			if !assert.NotNil(t, resp) {
				return
			}
			if !testCase.NotFound {
				assert.Equal(t, http.StatusOK, resp.StatusCode)
				return
			}
			if !assert.Equal(t, http.StatusNotFound, resp.StatusCode) {
				return
			}
			defer resp.Body.Close()

			b, err := io.ReadAll(resp.Body)
			if !assert.Nil(t, err) {
				return
			}

			var m map[string]any
			err = json.Unmarshal(b, &m)
			if !assert.Nil(t, err) {
				return
			}
			if !assert.Contains(t, m, "hello") {
				return
			}
			if !assert.Equal(t, "world", m["hello"]) {
				return
			}
		})
	}
}

func TestApp(t *testing.T) {
	t.Run("will return OpenAPI spec", func(t *testing.T) {
		t.Run("if a request is sent to /openapi.json", func(t *testing.T) {
			addrCh := make(chan net.Addr)
			app := NewApp(
				func(a *App) {
					a.listen = func(network, addr string) (net.Listener, error) {
						ls, err := net.Listen(network, ":0")
						if err != nil {
							return nil, err
						}
						defer close(addrCh)

						addrCh <- ls.Addr()
						return ls, nil
					}
				},
			)

			respCh := make(chan *http.Response, 1)
			ctx, cancel := context.WithCancel(context.Background())
			eg, egctx := errgroup.WithContext(ctx)
			eg.Go(func() error {
				return app.Run(egctx)
			})
			eg.Go(func() error {
				defer cancel()
				defer close(respCh)

				addr := <-addrCh
				resp, err := http.Get(fmt.Sprintf("http://%s/openapi.json", addr))
				if err != nil {
					return err
				}

				select {
				case <-egctx.Done():
					return egctx.Err()
				case respCh <- resp:
				}
				return nil
			})

			err := eg.Wait()
			if !assert.Nil(t, err) {
				return
			}

			resp := <-respCh
			if !assert.NotNil(t, resp) {
				return
			}
			defer resp.Body.Close()

			b, err := io.ReadAll(resp.Body)
			if !assert.Nil(t, err) {
				return
			}

			var spec openapi3.Spec
			err = json.Unmarshal(b, &spec)
			if !assert.Nil(t, err) {
				return
			}
			if !assert.Equal(t, "3.0.3", spec.Openapi) {
				return
			}
		})
	})
}

func TestApp_Run(t *testing.T) {
	t.Run("will return an error", func(t *testing.T) {
		t.Run("if it fails to marshal the OpenAPI spec to JSON", func(t *testing.T) {
			app := NewApp()

			marshalErr := errors.New("failed to marshal")
			app.marshalJSON = func(a any) ([]byte, error) {
				return nil, marshalErr
			}

			err := app.Run(context.Background())
			if !assert.ErrorIs(t, err, marshalErr) {
				return
			}
		})

		t.Run("if it fails to create a listener", func(t *testing.T) {
			app := NewApp()

			listenErr := errors.New("failed to listen")
			app.listen = func(network, addr string) (net.Listener, error) {
				return nil, listenErr
			}

			err := app.Run(context.Background())
			if !assert.ErrorIs(t, err, listenErr) {
				return
			}
		})
	})

	t.Run("will not return an error", func(t *testing.T) {
		t.Run("if the context.Context is cancelled", func(t *testing.T) {
			app := NewApp(ListenOn(0))

			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			err := app.Run(ctx)
			if !assert.Nil(t, err) {
				return
			}
		})
	})
}
