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
	"strings"
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

func (x *JsonContent) UnmarshalBinary(b []byte) error {
	return json.Unmarshal(b, x)
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

type FailUnmarshalBinary struct{}

var errUnmarshalBinary = errors.New("failed to unmarshal from binary")

func (*FailUnmarshalBinary) UnmarshalBinary(b []byte) error {
	return errUnmarshalBinary
}

type InvalidRequest struct{}

func (*InvalidRequest) UnmarshalBinary(b []byte) error {
	return nil
}

var errInvalidRequest = errors.New("invalid request")

func (InvalidRequest) Validate() error {
	return errInvalidRequest
}

type FailMarshalBinary struct{}

var errMarshalBinary = errors.New("failed to marshal to binary")

func (FailMarshalBinary) MarshalBinary() ([]byte, error) {
	return nil, errMarshalBinary
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
		t.Run("if the underlying Handler returns an error", func(t *testing.T) {
			pattern := "/"

			e := Get(
				pattern,
				HandlerFunc[Empty, Empty](func(_ context.Context, _ Empty) (Empty, error) {
					return Empty{}, errors.New("failed")
				}),
			)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, pattern, nil)

			e.ServeHTTP(w, r)

			resp := w.Result()
			if !assert.Equal(t, DefaultErrorStatusCode, resp.StatusCode) {
				return
			}
		})

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

		t.Run("if a required query param is missing", func(t *testing.T) {
			pattern := "/"

			e := Get(
				pattern,
				noopHandler{},
				QueryParams(
					QueryParam{
						Name:     "test",
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

		t.Run("if a query param does not match its expected pattern", func(t *testing.T) {
			pattern := "/"

			e := Get(
				pattern,
				noopHandler{},
				QueryParams(
					QueryParam{
						Name:    "test",
						Pattern: "^[a-zA-Z]*$",
					},
				),
			)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, pattern+"?test=abc123", nil)

			e.ServeHTTP(w, r)

			resp := w.Result()
			if !assert.Equal(t, http.StatusBadRequest, resp.StatusCode) {
				return
			}
		})

		t.Run("if the request content type header does not match the content type from ContentTyper", func(t *testing.T) {
			pattern := "/"

			e := Get(
				pattern,
				HandlerFunc[JsonContent, Empty](func(_ context.Context, _ JsonContent) (Empty, error) {
					return Empty{}, nil
				}),
			)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, pattern, nil)
			r.Header.Add("Content-Type", "application/xml")

			e.ServeHTTP(w, r)

			resp := w.Result()
			if !assert.Equal(t, http.StatusBadRequest, resp.StatusCode) {
				return
			}
		})

		t.Run("if the request body fails to unmarshal", func(t *testing.T) {
			pattern := "/"

			var caughtError error
			e := Post(
				pattern,
				HandlerFunc[FailUnmarshalBinary, Empty](func(_ context.Context, _ FailUnmarshalBinary) (Empty, error) {
					return Empty{}, nil
				}),
				OnError(errorHandlerFunc(func(w http.ResponseWriter, err error) {
					caughtError = err

					w.WriteHeader(DefaultErrorStatusCode)
				})),
			)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, pattern, strings.NewReader(`{}`))

			e.ServeHTTP(w, r)

			resp := w.Result()
			if !assert.Equal(t, DefaultErrorStatusCode, resp.StatusCode) {
				return
			}
			if !assert.Equal(t, errUnmarshalBinary, caughtError) {
				return
			}
		})

		t.Run("if the unmarshaled request body is invalid", func(t *testing.T) {
			pattern := "/"

			var caughtError error
			e := Post(
				pattern,
				HandlerFunc[InvalidRequest, Empty](func(_ context.Context, _ InvalidRequest) (Empty, error) {
					return Empty{}, nil
				}),
				OnError(errorHandlerFunc(func(w http.ResponseWriter, err error) {
					caughtError = err

					w.WriteHeader(DefaultErrorStatusCode)
				})),
			)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, pattern, strings.NewReader(`{}`))

			e.ServeHTTP(w, r)

			resp := w.Result()
			if !assert.Equal(t, DefaultErrorStatusCode, resp.StatusCode) {
				return
			}
			if !assert.Equal(t, errInvalidRequest, caughtError) {
				return
			}
		})

		t.Run("if the response body fails to marshal itself to binary", func(t *testing.T) {
			pattern := "/"

			var caughtError error
			e := Post(
				pattern,
				HandlerFunc[Empty, FailMarshalBinary](func(_ context.Context, _ Empty) (FailMarshalBinary, error) {
					return FailMarshalBinary{}, nil
				}),
				OnError(errorHandlerFunc(func(w http.ResponseWriter, err error) {
					caughtError = err

					w.WriteHeader(DefaultErrorStatusCode)
				})),
			)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, pattern, strings.NewReader(`{}`))

			e.ServeHTTP(w, r)

			resp := w.Result()
			if !assert.Equal(t, DefaultErrorStatusCode, resp.StatusCode) {
				return
			}
			if !assert.Equal(t, errMarshalBinary, caughtError) {
				return
			}
		})
	})
}
