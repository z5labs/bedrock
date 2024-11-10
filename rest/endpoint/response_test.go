// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package endpoint

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

func TestProducesJson(t *testing.T) {
	t.Run("will return an error", func(t *testing.T) {
		t.Run("if the inner handler returns an error", func(t *testing.T) {
			handleErr := errors.New("failed to handle request")
			h := HandlerFunc[EmptyRequest, EmptyResponse](func(ctx context.Context, req *EmptyRequest) (*EmptyResponse, error) {
				return nil, handleErr
			})

			var caughtError error
			op := NewOperation(
				ProducesJson(h),
				OnError(errorHandlerFunc(func(ctx context.Context, w http.ResponseWriter, err error) {
					caughtError = err

					w.WriteHeader(DefaultErrorStatusCode)
				})),
			)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/", nil)

			op.ServeHTTP(w, r)

			resp := w.Result()
			if !assert.Equal(t, DefaultErrorStatusCode, resp.StatusCode) {
				return
			}
			if !assert.Equal(t, handleErr, caughtError) {
				return
			}
		})
	})

	t.Run("will return json", func(t *testing.T) {
		t.Run("if the inner type successfully marshals to json", func(t *testing.T) {
			type echo struct {
				Msg string `json:"msg"`
			}

			h := HandlerFunc[EmptyRequest, echo](func(ctx context.Context, req *EmptyRequest) (*echo, error) {
				return &echo{Msg: "hello world"}, nil
			})

			var caughtError error
			op := NewOperation(
				ProducesJson(h),
				OnError(errorHandlerFunc(func(ctx context.Context, w http.ResponseWriter, err error) {
					caughtError = err

					w.WriteHeader(DefaultErrorStatusCode)
				})),
			)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/", nil)

			op.ServeHTTP(w, r)

			resp := w.Result()
			if !assert.Equal(t, DefaultStatusCode, resp.StatusCode) {
				return
			}
			if !assert.Nil(t, caughtError) {
				return
			}
			defer resp.Body.Close()

			b, err := io.ReadAll(resp.Body)
			if !assert.Nil(t, err) {
				return
			}

			var echoResp echo
			err = json.Unmarshal(b, &echoResp)
			if !assert.Nil(t, err) {
				return
			}
			if !assert.Equal(t, "hello world", echoResp.Msg) {
				return
			}
		})
	})
}

func TestProducesYaml(t *testing.T) {
	t.Run("will return an error", func(t *testing.T) {
		t.Run("if the inner handler returns an error", func(t *testing.T) {
			handleErr := errors.New("failed to handle request")
			h := HandlerFunc[EmptyRequest, EmptyResponse](func(ctx context.Context, req *EmptyRequest) (*EmptyResponse, error) {
				return nil, handleErr
			})

			var caughtError error
			op := NewOperation(
				ProducesYaml(h),
				OnError(errorHandlerFunc(func(ctx context.Context, w http.ResponseWriter, err error) {
					caughtError = err

					w.WriteHeader(DefaultErrorStatusCode)
				})),
			)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/", nil)

			op.ServeHTTP(w, r)

			resp := w.Result()
			if !assert.Equal(t, DefaultErrorStatusCode, resp.StatusCode) {
				return
			}
			if !assert.Equal(t, handleErr, caughtError) {
				return
			}
		})
	})

	t.Run("will return yaml", func(t *testing.T) {
		t.Run("if the inner type successfully marshals to yaml", func(t *testing.T) {
			type echo struct {
				Msg string `yaml:"msg"`
			}

			h := HandlerFunc[EmptyRequest, echo](func(ctx context.Context, req *EmptyRequest) (*echo, error) {
				return &echo{Msg: "hello world"}, nil
			})

			var caughtError error
			op := NewOperation(
				ProducesYaml(h),
				OnError(errorHandlerFunc(func(ctx context.Context, w http.ResponseWriter, err error) {
					caughtError = err

					w.WriteHeader(DefaultErrorStatusCode)
				})),
			)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/", nil)

			op.ServeHTTP(w, r)

			resp := w.Result()
			if !assert.Equal(t, DefaultStatusCode, resp.StatusCode) {
				return
			}
			if !assert.Nil(t, caughtError) {
				return
			}
			defer resp.Body.Close()

			b, err := io.ReadAll(resp.Body)
			if !assert.Nil(t, err) {
				return
			}

			var echoResp echo
			err = yaml.Unmarshal(b, &echoResp)
			if !assert.Nil(t, err) {
				return
			}
			if !assert.Equal(t, "hello world", echoResp.Msg) {
				return
			}
		})
	})
}
