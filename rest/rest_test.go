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
	"path"
	"testing"

	"github.com/z5labs/bedrock/pkg/ptr"
	"github.com/z5labs/bedrock/rest/mux"

	"github.com/stretchr/testify/assert"
	"github.com/swaggest/openapi-go/openapi3"
	"golang.org/x/sync/errgroup"
	"gopkg.in/yaml.v3"
)

type statusCodeHandler int

func (h statusCodeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(int(h))
}

func (h statusCodeHandler) OpenApi() openapi3.Operation {
	return openapi3.Operation{}
}

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

			mux := mux.NewHttp(
				mux.NotFoundHandler(notFoundHandler),
			)

			ls, err := net.Listen("tcp", ":0")
			if !assert.Nil(t, err) {
				return
			}

			app := NewApp(
				Listener(ls),
				WithMux(mux),
				func(a *App) {
					if testCase.RegisterPattern == "" {
						return
					}
					Endpoint(http.MethodGet, testCase.RegisterPattern, statusCodeHandler(http.StatusOK))(a)
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

				addr := ls.Addr()
				url := fmt.Sprintf("http://%s", path.Join(addr.String(), testCase.RequestPath))
				resp, err := http.Get(url)
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

			err = eg.Wait()
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

func TestMethodNotAllowedHandler(t *testing.T) {
	testCases := []struct {
		Name             string
		RegisterPatterns map[mux.Method]string
		Method           mux.Method
		RequestPath      string
		MethodNotAllowed bool
	}{
		{
			Name: "should return success response when correct method is used",
			RegisterPatterns: map[mux.Method]string{
				http.MethodGet: "/",
			},
			Method:           mux.MethodGet,
			RequestPath:      "/",
			MethodNotAllowed: false,
		},
		{
			Name: "should return success response when more than one method is registered for same path",
			RegisterPatterns: map[mux.Method]string{
				http.MethodGet:  "/",
				http.MethodPost: "/",
			},
			Method:           mux.MethodGet,
			RequestPath:      "/",
			MethodNotAllowed: false,
		},
		{
			Name: "should return method not allowed response when incorrect method is used",
			RegisterPatterns: map[mux.Method]string{
				http.MethodGet: "/",
			},
			Method:           mux.MethodPost,
			RequestPath:      "/",
			MethodNotAllowed: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.Name, func(t *testing.T) {
			methodNotAllowedHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusMethodNotAllowed)

				enc := json.NewEncoder(w)
				enc.Encode(map[string]any{"hello": "world"})
			})

			mux := mux.NewHttp(
				mux.MethodNotAllowedHandler(methodNotAllowedHandler),
			)

			ls, err := net.Listen("tcp", ":0")
			if !assert.Nil(t, err) {
				return
			}

			app := NewApp(
				Listener(ls),
				WithMux(mux),
				func(a *App) {
					for method, pattern := range testCase.RegisterPatterns {
						Endpoint(method, pattern, statusCodeHandler(http.StatusOK))(a)
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

				addr := ls.Addr()
				url := fmt.Sprintf("http://%s", path.Join(addr.String(), testCase.RequestPath))

				req, err := http.NewRequestWithContext(egctx, string(testCase.Method), url, nil)
				if err != nil {
					return err
				}

				resp, err := http.DefaultClient.Do(req)
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

			err = eg.Wait()
			if !assert.Nil(t, err) {
				return
			}

			resp := <-respCh
			if !assert.NotNil(t, resp) {
				return
			}
			if !testCase.MethodNotAllowed {
				assert.Equal(t, http.StatusOK, resp.StatusCode)
				return
			}
			if !assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode) {
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

func TestOpenApiJsonHandler(t *testing.T) {
	t.Run("will return HTTP 500 status code", func(t *testing.T) {
		t.Run("if the json marshalling fails", func(t *testing.T) {
			ls, err := net.Listen("tcp", ":0")
			if !assert.Nil(t, err) {
				return
			}

			app := NewApp(
				Listener(ls),
				OpenApiEndpoint(http.MethodGet, "/openapi.json", func(s *openapi3.Spec) http.Handler {
					return openApiHandler{
						spec: s,
						marshal: func(a any) ([]byte, error) {
							return nil, errors.New("failed to marshal")
						},
					}
				}),
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

				addr := ls.Addr()
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

			err = eg.Wait()
			if !assert.Nil(t, err) {
				return
			}

			resp := <-respCh
			if !assert.NotNil(t, resp) {
				return
			}
			defer resp.Body.Close()

			if !assert.Equal(t, http.StatusInternalServerError, resp.StatusCode) {
				return
			}
		})
	})

	t.Run("will return OpenAPI spec", func(t *testing.T) {
		t.Run("if a GET request is sent to /openapi.json", func(t *testing.T) {
			ls, err := net.Listen("tcp", ":0")
			if !assert.Nil(t, err) {
				return
			}

			app := NewApp(
				Listener(ls),
				OpenApiEndpoint(http.MethodGet, "/openapi.json", OpenApiJsonHandler),
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

				addr := ls.Addr()
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

			err = eg.Wait()
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

func TestOpenApiYamlHandler(t *testing.T) {
	t.Run("will return a HTTP 500 status code", func(t *testing.T) {
		t.Run("if the yaml marshalling fails", func(t *testing.T) {
			ls, err := net.Listen("tcp", ":0")
			if !assert.Nil(t, err) {
				return
			}

			app := NewApp(
				Listener(ls),
				OpenApiEndpoint(http.MethodGet, "/openapi.yaml", func(s *openapi3.Spec) http.Handler {
					return openApiHandler{
						spec: s,
						marshal: func(a any) ([]byte, error) {
							return nil, errors.New("failed to marshal")
						},
					}
				}),
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

				addr := ls.Addr()
				resp, err := http.Get(fmt.Sprintf("http://%s/openapi.yaml", addr))
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

			err = eg.Wait()
			if !assert.Nil(t, err) {
				return
			}

			resp := <-respCh
			if !assert.NotNil(t, resp) {
				return
			}
			defer resp.Body.Close()

			if !assert.Equal(t, http.StatusInternalServerError, resp.StatusCode) {
				return
			}
		})
	})

	t.Run("will return OpenAPI spec", func(t *testing.T) {
		t.Run("if a GET request is sent to /openapi.yaml", func(t *testing.T) {
			ls, err := net.Listen("tcp", ":0")
			if !assert.Nil(t, err) {
				return
			}

			app := NewApp(
				Listener(ls),
				OpenApiEndpoint(http.MethodGet, "/openapi.yaml", OpenApiYamlHandler),
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

				addr := ls.Addr()
				resp, err := http.Get(fmt.Sprintf("http://%s/openapi.yaml", addr))
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

			err = eg.Wait()
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
			err = yaml.Unmarshal(b, &spec)
			if !assert.Nil(t, err) {
				return
			}
			if !assert.Equal(t, "3.0.3", spec.Openapi) {
				return
			}
		})
	})
}

type operationHandler openapi3.Operation

func (h operationHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {}

func (h operationHandler) OpenApi() openapi3.Operation {
	return (openapi3.Operation)(h)
}

func TestApp_Run(t *testing.T) {
	t.Run("will return an error", func(t *testing.T) {
		t.Run("if it fails to create the default net.Listener", func(t *testing.T) {
			app := NewApp()

			listenErr := errors.New("failed to listen")
			app.listen = func(network, addr string) (net.Listener, error) {
				return nil, listenErr
			}

			err := app.Run(context.Background())
			if !assert.Equal(t, listenErr, err) {
				return
			}
		})

		t.Run("if a path parameter defined in the openapi3.Operation is not in the path pattern", func(t *testing.T) {
			h := operationHandler(openapi3.Operation{
				Parameters: []openapi3.ParameterOrRef{
					{
						Parameter: &openapi3.Parameter{
							In:   openapi3.ParameterInPath,
							Name: "id",
							Schema: &openapi3.SchemaOrRef{
								Schema: &openapi3.Schema{
									Type: ptr.Ref(openapi3.SchemaTypeString),
								},
							},
						},
					},
				},
			})

			ls, err := net.Listen("tcp", ":0")
			if !assert.Nil(t, err) {
				return
			}

			app := NewApp(
				Listener(ls),
				Endpoint(http.MethodGet, "/", h),
			)

			err = app.Run(context.Background())
			if !assert.NotNil(t, err) {
				return
			}
		})
	})

	t.Run("will not return an error", func(t *testing.T) {
		t.Run("if the context.Context is cancelled", func(t *testing.T) {
			ls, err := net.Listen("tcp", ":0")
			if !assert.Nil(t, err) {
				return
			}

			app := NewApp(Listener(ls))

			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			err = app.Run(ctx)
			if !assert.Nil(t, err) {
				return
			}
		})

		t.Run("if a path parameter defined in the openapi3.Operation is in the path pattern", func(t *testing.T) {
			h := operationHandler(openapi3.Operation{
				Parameters: []openapi3.ParameterOrRef{
					{
						Parameter: &openapi3.Parameter{
							In:   openapi3.ParameterInPath,
							Name: "id",
							Schema: &openapi3.SchemaOrRef{
								Schema: &openapi3.Schema{
									Type: ptr.Ref(openapi3.SchemaTypeString),
								},
							},
						},
					},
				},
			})

			ls, err := net.Listen("tcp", ":0")
			if !assert.Nil(t, err) {
				return
			}

			app := NewApp(
				Listener(ls),
				Endpoint(http.MethodGet, "/{id}", h),
			)

			ctx, cancel := context.WithCancel(context.Background())
			eg, egctx := errgroup.WithContext(ctx)
			eg.Go(func() error {
				return app.Run(egctx)
			})

			respCh := make(chan *http.Response, 1)
			eg.Go(func() error {
				defer cancel()
				defer close(respCh)

				req, err := http.NewRequestWithContext(
					egctx,
					http.MethodGet,
					fmt.Sprintf("http://%s/123", ls.Addr()),
					nil,
				)
				if err != nil {
					return err
				}

				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					return err
				}

				respCh <- resp
				return nil
			})

			err = eg.Wait()
			if !assert.Nil(t, err) {
				return
			}

			resp := <-respCh
			if !assert.NotNil(t, resp) {
				return
			}
			if !assert.Equal(t, http.StatusOK, resp.StatusCode) {
				return
			}
		})

		t.Run("if a wildcard path parameter is used", func(t *testing.T) {
			h := operationHandler(openapi3.Operation{
				Parameters: []openapi3.ParameterOrRef{
					{
						Parameter: &openapi3.Parameter{
							In:   openapi3.ParameterInPath,
							Name: "id",
							Schema: &openapi3.SchemaOrRef{
								Schema: &openapi3.Schema{
									Type: ptr.Ref(openapi3.SchemaTypeString),
								},
							},
						},
					},
				},
			})

			ls, err := net.Listen("tcp", ":0")
			if !assert.Nil(t, err) {
				return
			}

			app := NewApp(
				Listener(ls),
				Endpoint(http.MethodGet, "/{id...}", h),
			)

			ctx, cancel := context.WithCancel(context.Background())
			eg, egctx := errgroup.WithContext(ctx)
			eg.Go(func() error {
				return app.Run(egctx)
			})

			respCh := make(chan *http.Response, 1)
			eg.Go(func() error {
				defer cancel()
				defer close(respCh)

				req, err := http.NewRequestWithContext(
					egctx,
					http.MethodGet,
					fmt.Sprintf("http://%s/123", ls.Addr()),
					nil,
				)
				if err != nil {
					return err
				}

				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					return err
				}

				respCh <- resp
				return nil
			})

			err = eg.Wait()
			if !assert.Nil(t, err) {
				return
			}

			resp := <-respCh
			if !assert.NotNil(t, resp) {
				return
			}
			if !assert.Equal(t, http.StatusOK, resp.StatusCode) {
				return
			}
		})
	})
}
