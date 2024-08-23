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

type Empty struct{}

type noopHandler struct{}

func (noopHandler) Handle(_ context.Context, _ *Empty) (*Empty, error) {
	return &Empty{}, nil
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
			e := NewOperation(noopHandler{})

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/", nil)

			e.ServeHTTP(w, r)

			resp := w.Result()
			if !assert.Equal(t, DefaultStatusCode, resp.StatusCode) {
				return
			}
		})

		t.Run("if the underlying Handler succeeds with a encoding.BinaryMarshaler response", func(t *testing.T) {
			e := NewOperation(
				HandlerFunc[Empty, JsonContent](func(_ context.Context, _ *Empty) (*JsonContent, error) {
					return &JsonContent{Value: "hello, world"}, nil
				}),
			)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/", nil)

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

	t.Run("will inject path params", func(t *testing.T) {
		t.Run("if a valid http.ServeMux path param pattern is used", func(t *testing.T) {
			e := NewOperation(
				HandlerFunc[Empty, JsonContent](func(ctx context.Context, _ *Empty) (*JsonContent, error) {
					v := PathValue(ctx, "id")
					return &JsonContent{Value: v}, nil
				}),
				PathParams(PathParam{
					Name: "id",
				}),
			)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/abc123", nil)

			// for path params a http.ServeMux must be used since
			// Endpoint doesn't support it directly
			mux := http.NewServeMux()
			mux.Handle("GET /{id}", e)
			mux.ServeHTTP(w, r)

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
			if !assert.Equal(t, "abc123", jsonResp.Value) {
				return
			}
		})
	})

	t.Run("will inject headers", func(t *testing.T) {
		t.Run("if a header is configured with the Headers option", func(t *testing.T) {
			e := NewOperation(
				HandlerFunc[Empty, JsonContent](func(ctx context.Context, _ *Empty) (*JsonContent, error) {
					v := HeaderValue(ctx, "test-header")
					return &JsonContent{Value: v}, nil
				}),
				Headers(Header{
					Name: "test-header",
				}),
			)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.Header.Set("test-header", "hello, world")

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

	t.Run("will inject query params", func(t *testing.T) {
		t.Run("if a query param is configured with the QueryParams option", func(t *testing.T) {
			e := NewOperation(
				HandlerFunc[Empty, JsonContent](func(ctx context.Context, _ *Empty) (*JsonContent, error) {
					v := QueryValue(ctx, "test-query")
					return &JsonContent{Value: v}, nil
				}),
				QueryParams(QueryParam{
					Name: "test-query",
				}),
			)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/?test-query=abc123", nil)

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
			if !assert.Equal(t, "abc123", jsonResp.Value) {
				return
			}
		})
	})

	t.Run("will return custom success http status code", func(t *testing.T) {
		t.Run("if the StatusCode option is used and the underlying Handler succeeds with an empty response", func(t *testing.T) {
			statusCode := http.StatusCreated
			if !assert.NotEqual(t, DefaultStatusCode, statusCode) {
				return
			}

			e := NewOperation(
				noopHandler{},
				StatusCode(statusCode),
			)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/", nil)

			e.ServeHTTP(w, r)

			resp := w.Result()
			if !assert.Equal(t, statusCode, resp.StatusCode) {
				return
			}
		})

		t.Run("if the StatusCode option is used and the underlying Handler succeeds with a encoding.BinaryMarshaler response", func(t *testing.T) {
			statusCode := http.StatusCreated
			if !assert.NotEqual(t, DefaultStatusCode, statusCode) {
				return
			}

			e := NewOperation(
				HandlerFunc[Empty, JsonContent](func(_ context.Context, _ *Empty) (*JsonContent, error) {
					return &JsonContent{Value: "hello, world"}, nil
				}),
				StatusCode(statusCode),
			)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/", nil)

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
			e := NewOperation(
				HandlerFunc[Empty, Empty](func(_ context.Context, _ *Empty) (*Empty, error) {
					return nil, errors.New("failed")
				}),
			)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/", nil)

			e.ServeHTTP(w, r)

			resp := w.Result()
			if !assert.Equal(t, DefaultErrorStatusCode, resp.StatusCode) {
				return
			}
		})

		t.Run("if a custom error handler is set", func(t *testing.T) {
			errStatusCode := http.StatusServiceUnavailable
			if !assert.NotEqual(t, DefaultErrorStatusCode, errStatusCode) {
				return
			}

			e := NewOperation(
				HandlerFunc[Empty, Empty](func(_ context.Context, _ *Empty) (*Empty, error) {
					return nil, errors.New("failed")
				}),
				OnError(errorHandlerFunc(func(w http.ResponseWriter, err error) {
					w.WriteHeader(errStatusCode)
				})),
			)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/", nil)

			e.ServeHTTP(w, r)

			resp := w.Result()
			if !assert.Equal(t, errStatusCode, resp.StatusCode) {
				return
			}
		})

		t.Run("if a required http header is missing", func(t *testing.T) {
			var caughtError error
			e := NewOperation(
				noopHandler{},
				Headers(
					Header{
						Name:     "Authorization",
						Required: true,
					},
				),
				OnError(errorHandlerFunc(func(w http.ResponseWriter, err error) {
					caughtError = err

					w.WriteHeader(DefaultErrorStatusCode)
				})),
			)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/", nil)

			e.ServeHTTP(w, r)

			resp := w.Result()
			if !assert.Equal(t, DefaultErrorStatusCode, resp.StatusCode) {
				return
			}

			var herr MissingRequiredHeaderError
			if !assert.ErrorAs(t, caughtError, &herr) {
				return
			}
			if !assert.NotEmpty(t, herr.Error()) {
				return
			}
		})

		t.Run("if a http header does not match its expected pattern", func(t *testing.T) {
			var caughtError error
			e := NewOperation(
				noopHandler{},
				Headers(
					Header{
						Name:    "Authorization",
						Pattern: "^[a-zA-Z]*$",
					},
				),
				OnError(errorHandlerFunc(func(w http.ResponseWriter, err error) {
					caughtError = err

					w.WriteHeader(DefaultErrorStatusCode)
				})),
			)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.Header.Set("Authorization", "abc123")

			e.ServeHTTP(w, r)

			resp := w.Result()
			if !assert.Equal(t, DefaultErrorStatusCode, resp.StatusCode) {
				return
			}

			var herr InvalidHeaderError
			if !assert.ErrorAs(t, caughtError, &herr) {
				return
			}
			if !assert.NotEmpty(t, herr.Error()) {
				return
			}
		})

		t.Run("if a required query param is missing", func(t *testing.T) {
			var caughtError error
			e := NewOperation(
				noopHandler{},
				QueryParams(
					QueryParam{
						Name:     "test",
						Required: true,
					},
				),
				OnError(errorHandlerFunc(func(w http.ResponseWriter, err error) {
					caughtError = err

					w.WriteHeader(DefaultErrorStatusCode)
				})),
			)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/", nil)

			e.ServeHTTP(w, r)

			resp := w.Result()
			if !assert.Equal(t, DefaultErrorStatusCode, resp.StatusCode) {
				return
			}

			var qerr MissingRequiredQueryParamError
			if !assert.ErrorAs(t, caughtError, &qerr) {
				return
			}
			if !assert.NotEmpty(t, qerr.Error()) {
				return
			}
		})

		t.Run("if a query param does not match its expected pattern", func(t *testing.T) {
			var caughtError error
			e := NewOperation(
				noopHandler{},
				QueryParams(
					QueryParam{
						Name:    "test",
						Pattern: "^[a-zA-Z]*$",
					},
				),
				OnError(errorHandlerFunc(func(w http.ResponseWriter, err error) {
					caughtError = err

					w.WriteHeader(DefaultErrorStatusCode)
				})),
			)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/?test=abc123", nil)

			e.ServeHTTP(w, r)

			resp := w.Result()
			if !assert.Equal(t, DefaultErrorStatusCode, resp.StatusCode) {
				return
			}

			var qerr InvalidQueryParamError
			if !assert.ErrorAs(t, caughtError, &qerr) {
				return
			}
			if !assert.NotEmpty(t, qerr.Error()) {
				return
			}
		})

		t.Run("if the request content type header does not match the content type from ContentTyper", func(t *testing.T) {
			var caughtError error
			e := NewOperation(
				HandlerFunc[JsonContent, Empty](func(_ context.Context, _ *JsonContent) (*Empty, error) {
					return &Empty{}, nil
				}),
				OnError(errorHandlerFunc(func(w http.ResponseWriter, err error) {
					caughtError = err

					w.WriteHeader(DefaultErrorStatusCode)
				})),
			)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.Header.Add("Content-Type", "application/xml")

			e.ServeHTTP(w, r)

			resp := w.Result()
			if !assert.Equal(t, DefaultErrorStatusCode, resp.StatusCode) {
				return
			}

			var herr InvalidHeaderError
			if !assert.ErrorAs(t, caughtError, &herr) {
				return
			}
			if !assert.NotEmpty(t, herr.Error()) {
				return
			}
		})

		t.Run("if the request body fails to unmarshal", func(t *testing.T) {
			var caughtError error
			e := NewOperation(
				HandlerFunc[FailUnmarshalBinary, Empty](func(_ context.Context, _ *FailUnmarshalBinary) (*Empty, error) {
					return &Empty{}, nil
				}),
				OnError(errorHandlerFunc(func(w http.ResponseWriter, err error) {
					caughtError = err

					w.WriteHeader(DefaultErrorStatusCode)
				})),
			)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`))

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
			var caughtError error
			e := NewOperation(
				HandlerFunc[InvalidRequest, Empty](func(_ context.Context, _ *InvalidRequest) (*Empty, error) {
					return &Empty{}, nil
				}),
				OnError(errorHandlerFunc(func(w http.ResponseWriter, err error) {
					caughtError = err

					w.WriteHeader(DefaultErrorStatusCode)
				})),
			)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`))

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
			var caughtError error
			e := NewOperation(
				HandlerFunc[Empty, FailMarshalBinary](func(_ context.Context, _ *Empty) (*FailMarshalBinary, error) {
					return &FailMarshalBinary{}, nil
				}),
				OnError(errorHandlerFunc(func(w http.ResponseWriter, err error) {
					caughtError = err

					w.WriteHeader(DefaultErrorStatusCode)
				})),
			)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`))

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

	t.Run("will return response header", func(t *testing.T) {
		t.Run("if the response body implements ContentTyper", func(t *testing.T) {
			e := NewOperation(
				HandlerFunc[Empty, JsonContent](func(_ context.Context, _ *Empty) (*JsonContent, error) {
					return &JsonContent{Value: "hello, world"}, nil
				}),
			)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/", nil)

			e.ServeHTTP(w, r)

			resp := w.Result()
			if !assert.Equal(t, DefaultStatusCode, resp.StatusCode) {
				return
			}
			if !assert.Equal(t, JsonContent{}.ContentType(), resp.Header.Get("Content-Type")) {
				return
			}

			b, err := io.ReadAll(resp.Body)
			if !assert.Nil(t, err) {
				return
			}

			var content JsonContent
			err = json.Unmarshal(b, &content)
			if !assert.Nil(t, err) {
				return
			}
			if !assert.Equal(t, "hello, world", content.Value) {
				return
			}
		})

		t.Run("if the underlying Handler sets a custom response header using the context", func(t *testing.T) {
			e := NewOperation(
				HandlerFunc[Empty, Empty](func(ctx context.Context, _ *Empty) (*Empty, error) {
					SetResponseHeader(ctx, "Content-Type", "test-content-type")
					return &Empty{}, nil
				}),
			)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/", nil)

			e.ServeHTTP(w, r)

			resp := w.Result()
			if !assert.Equal(t, DefaultStatusCode, resp.StatusCode) {
				return
			}
			if !assert.Equal(t, "test-content-type", resp.Header.Get("Content-Type")) {
				return
			}
		})
	})
}
