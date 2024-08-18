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
)

type noopHandler struct{}

func (noopHandler) Handle(_ context.Context, _ Empty) (Empty, error) {
	return Empty{}, nil
}

type JsonContent struct {
	Value string `json:"value"`
}

func (JsonContent) ContentType() string {
	return "application/json"
}

func (x JsonContent) MarshalBinary() ([]byte, error) {
	return json.Marshal(x)
}

type httpError struct {
	status int
}

func (httpError) Error() string {
	return ""
}

func (e httpError) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(e.status)
}

func TestEndpoint_ServeHTTP(t *testing.T) {
	t.Run("will return the default success http status code", func(t *testing.T) {
		t.Run("if the underlying Handler succeeds with an empty response", func(t *testing.T) {
			pattern := "/"

			e := Get(
				pattern,
				noopHandler{},
			)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, pattern, nil)

			e.ServeHTTP(w, r)

			resp := w.Result()
			if !assert.Equal(t, DefaultStatusCode, resp.StatusCode) {
				return
			}
		})

		t.Run("if the underlying Handler succeeds with a encoding.BinaryMarshaler response", func(t *testing.T) {
			pattern := "/"

			e := Get(
				pattern,
				HandlerFunc[Empty, JsonContent](func(_ context.Context, _ Empty) (JsonContent, error) {
					return JsonContent{Value: "hello, world"}, nil
				}),
			)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, pattern, nil)

			e.ServeHTTP(w, r)

			resp := w.Result()
			if !assert.Equal(t, DefaultStatusCode, resp.StatusCode) {
				return
			}

			b, err := io.ReadAll(resp.Body)
			if !assert.Nil(t, err) {
				return
			}

			var jsonResp JsonContent
			err = json.Unmarshal(b, &jsonResp)
			if !assert.Nil(t, err) {
				return
			}
			if !assert.Equal(t, "hello, world", jsonResp.Value) {
				return
			}
		})
	})

	t.Run("will return custom success http status code", func(t *testing.T) {
		t.Run("if the StatusCode option is used and the underlying Handler succeeds with an empty response", func(t *testing.T) {
			pattern := "/"
			statusCode := http.StatusCreated
			if !assert.NotEqual(t, DefaultStatusCode, statusCode) {
				return
			}

			e := Get(
				pattern,
				noopHandler{},
				StatusCode(statusCode),
			)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, pattern, nil)

			e.ServeHTTP(w, r)

			resp := w.Result()
			if !assert.Equal(t, statusCode, resp.StatusCode) {
				return
			}
		})

		t.Run("if the StatusCode option is used and the underlying Handler succeeds with a encoding.BinaryMarshaler response", func(t *testing.T) {
			pattern := "/"
			statusCode := http.StatusCreated
			if !assert.NotEqual(t, DefaultStatusCode, statusCode) {
				return
			}

			e := Get(
				pattern,
				HandlerFunc[Empty, JsonContent](func(_ context.Context, _ Empty) (JsonContent, error) {
					return JsonContent{Value: "hello, world"}, nil
				}),
				StatusCode(statusCode),
			)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, pattern, nil)

			e.ServeHTTP(w, r)

			resp := w.Result()
			if !assert.Equal(t, statusCode, resp.StatusCode) {
				return
			}

			b, err := io.ReadAll(resp.Body)
			if !assert.Nil(t, err) {
				return
			}

			var jsonResp JsonContent
			err = json.Unmarshal(b, &jsonResp)
			if !assert.Nil(t, err) {
				return
			}
			if !assert.Equal(t, "hello, world", jsonResp.Value) {
				return
			}
		})
	})

	t.Run("will return non-success http status code", func(t *testing.T) {
		t.Run("if a custom error handler is set", func(t *testing.T) {
			pattern := "/"
			errStatusCode := http.StatusServiceUnavailable
			if !assert.NotEqual(t, DefaultErrorStatusCode, errStatusCode) {
				return
			}

			e := Get(
				pattern,
				HandlerFunc[Empty, Empty](func(_ context.Context, _ Empty) (Empty, error) {
					return Empty{}, errors.New("failed")
				}),
				OnError(errorHandlerFunc(func(w http.ResponseWriter, err error) {
					w.WriteHeader(errStatusCode)
				})),
			)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, pattern, nil)

			e.ServeHTTP(w, r)

			resp := w.Result()
			if !assert.Equal(t, errStatusCode, resp.StatusCode) {
				return
			}
		})

		t.Run("if the underlying error implements http.Handler", func(t *testing.T) {
			pattern := "/"
			errStatusCode := http.StatusServiceUnavailable
			if !assert.NotEqual(t, DefaultErrorStatusCode, errStatusCode) {
				return
			}

			e := Get(
				pattern,
				HandlerFunc[Empty, Empty](func(_ context.Context, _ Empty) (Empty, error) {
					return Empty{}, httpError{status: errStatusCode}
				}),
			)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, pattern, nil)

			e.ServeHTTP(w, r)

			resp := w.Result()
			if !assert.Equal(t, errStatusCode, resp.StatusCode) {
				return
			}
		})

		t.Run("if the http request is for the wrong http method", func(t *testing.T) {
			pattern := "/"

			e := Get(
				pattern,
				noopHandler{},
			)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, pattern, nil)

			e.ServeHTTP(w, r)

			resp := w.Result()
			if !assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode) {
				return
			}
		})

		t.Run("if a required http header is missing", func(t *testing.T) {
			pattern := "/"

			e := Get(
				pattern,
				noopHandler{},
				Headers(
					Header{
						Name:     "Authorization",
						Required: true,
					},
				),
			)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, pattern, nil)

			e.ServeHTTP(w, r)

			resp := w.Result()
			if !assert.Equal(t, http.StatusBadRequest, resp.StatusCode) {
				return
			}
		})

		t.Run("if a http header does not match its expected pattern", func(t *testing.T) {
			pattern := "/"

			e := Get(
				pattern,
				noopHandler{},
				Headers(
					Header{
						Name:    "Authorization",
						Pattern: "^[a-zA-Z]*$",
					},
				),
			)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, pattern, nil)
			r.Header.Set("Authorization", "abc123")

			e.ServeHTTP(w, r)

			resp := w.Result()
			if !assert.Equal(t, http.StatusBadRequest, resp.StatusCode) {
				return
			}
		})
	})
}
