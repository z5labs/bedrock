// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package endpoint

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/swaggest/openapi-go/openapi3"
)

type noopHandler[Req, Resp any] struct{}

func (noopHandler[Req, Resp]) Handle(_ context.Context, _ *Req) (*Resp, error) {
	var resp Resp
	return &resp, nil
}

type ReaderContent struct {
	r io.Reader
}

func (ReaderContent) ContentType() string {
	return "application/octet"
}

func (ReaderContent) Validate() error {
	return nil
}

func (ReaderContent) OpenApiV3Schema() (*openapi3.Schema, error) {
	return nil, nil
}

func (x *ReaderContent) ReadRequest(r *http.Request) (err error) {
	defer close(&err, r.Body)

	var b []byte
	b, err = io.ReadAll(r.Body)
	if err != nil {
		return
	}
	x.r = bytes.NewReader(b)
	return
}

func (x *ReaderContent) WriteTo(w io.Writer) (int64, error) {
	return io.Copy(w, x.r)
}

type FailReadFrom struct{}

var errReadFrom = errors.New("failed to read from io.Reader")

func (FailReadFrom) ContentType() string {
	return ""
}

func (FailReadFrom) Validate() error {
	return nil
}

func (FailReadFrom) OpenApiV3Schema() (*openapi3.Schema, error) {
	return nil, nil
}

func (*FailReadFrom) ReadRequest(r *http.Request) error {
	return errReadFrom
}

type InvalidRequest struct{}

func (InvalidRequest) ContentType() string {
	return ""
}

var errInvalidRequest = errors.New("invalid request")

func (InvalidRequest) Validate() error {
	return errInvalidRequest
}

func (InvalidRequest) OpenApiV3Schema() (*openapi3.Schema, error) {
	return nil, nil
}

func (*InvalidRequest) ReadRequest(r *http.Request) error {
	return nil
}

type FailWriteTo struct{}

func (FailWriteTo) ContentType() string {
	return ""
}

func (FailWriteTo) OpenApiV3Schema() (*openapi3.Schema, error) {
	return nil, nil
}

var errWriteTo = errors.New("failed to write response")

func (*FailWriteTo) WriteTo(w io.Writer) (int64, error) {
	return 0, errWriteTo
}

func TestEndpoint_ServeHTTP(t *testing.T) {
	t.Run("will return the default success http status code", func(t *testing.T) {
		t.Run("if the underlying Handler succeeds with an empty response", func(t *testing.T) {
			e := NewOperation(noopHandler[EmptyRequest, EmptyResponse]{})

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/", nil)

			e.ServeHTTP(w, r)

			resp := w.Result()
			if !assert.Equal(t, DefaultStatusCode, resp.StatusCode) {
				return
			}
		})

		t.Run("if the underlying Handler succeeds with a io.WriterTo response", func(t *testing.T) {
			e := NewOperation(
				HandlerFunc[ReaderContent, ReaderContent](func(_ context.Context, req *ReaderContent) (*ReaderContent, error) {
					return &ReaderContent{r: req.r}, nil
				}),
			)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("hello, world"))
			r.Header.Set("Content-Type", ReaderContent{}.ContentType())

			e.ServeHTTP(w, r)

			resp := w.Result()
			if !assert.Equal(t, DefaultStatusCode, resp.StatusCode) {
				return
			}

			b, err := io.ReadAll(resp.Body)
			if !assert.Nil(t, err) {
				return
			}
			if !assert.Equal(t, "hello, world", string(b)) {
				return
			}
		})

		t.Run("if the response fails to write itself to the http.ResponseWriter", func(t *testing.T) {
			var caughtError error
			e := NewOperation(
				HandlerFunc[EmptyRequest, FailWriteTo](func(_ context.Context, _ *EmptyRequest) (*FailWriteTo, error) {
					t.Log("request received")
					return &FailWriteTo{}, nil
				}),
				OnError(errorHandlerFunc(func(ctx context.Context, w http.ResponseWriter, err error) {
					caughtError = err

					w.WriteHeader(DefaultErrorStatusCode)
				})),
			)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`))

			e.ServeHTTP(w, r)

			resp := w.Result()
			if !assert.Equal(t, DefaultStatusCode, resp.StatusCode) {
				return
			}
			if !assert.Equal(t, errWriteTo, caughtError) {
				return
			}
		})
	})

	t.Run("will inject path params", func(t *testing.T) {
		t.Run("if a valid http.ServeMux path param pattern is used", func(t *testing.T) {
			type jsonContent struct {
				Value string `json:"value"`
			}

			e := NewOperation(
				ProducesJson(HandlerFunc[EmptyRequest, jsonContent](func(ctx context.Context, _ *EmptyRequest) (*jsonContent, error) {
					v := PathValue(ctx, "id")
					return &jsonContent{Value: v}, nil
				})),
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

			var jsonResp jsonContent
			err = json.Unmarshal(b, &jsonResp)
			if !assert.Nil(t, err) {
				return
			}
			if !assert.Equal(t, "abc123", jsonResp.Value) {
				return
			}
		})

		t.Run("if a valid http.ServeMux path param pattern is used and it's marked as required", func(t *testing.T) {
			type jsonContent struct {
				Value string `json:"value"`
			}

			e := NewOperation(
				ProducesJson(HandlerFunc[EmptyRequest, jsonContent](func(ctx context.Context, _ *EmptyRequest) (*jsonContent, error) {
					v := PathValue(ctx, "id")
					return &jsonContent{Value: v}, nil
				})),
				PathParams(PathParam{
					Name:     "id",
					Required: true,
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

			var jsonResp jsonContent
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
			type jsonContent struct {
				Value string `json:"value"`
			}

			e := NewOperation(
				ProducesJson(HandlerFunc[EmptyRequest, jsonContent](func(ctx context.Context, _ *EmptyRequest) (*jsonContent, error) {
					v := HeaderValue(ctx, "test-header")
					return &jsonContent{Value: v}, nil
				})),
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

			var jsonResp jsonContent
			err = json.Unmarshal(b, &jsonResp)
			if !assert.Nil(t, err) {
				return
			}
			if !assert.Equal(t, "hello, world", jsonResp.Value) {
				return
			}
		})

		t.Run("if a header is configured with the Headers option and marked as required", func(t *testing.T) {
			type jsonContent struct {
				Value string `json:"value"`
			}

			e := NewOperation(
				ProducesJson(HandlerFunc[EmptyRequest, jsonContent](func(ctx context.Context, _ *EmptyRequest) (*jsonContent, error) {
					v := HeaderValue(ctx, "test-header")
					return &jsonContent{Value: v}, nil
				})),
				Headers(Header{
					Name:     "test-header",
					Required: true,
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

			var jsonResp jsonContent
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
			type jsonContent struct {
				Value string `json:"value"`
			}

			e := NewOperation(
				ProducesJson(HandlerFunc[EmptyRequest, jsonContent](func(ctx context.Context, _ *EmptyRequest) (*jsonContent, error) {
					v := QueryValue(ctx, "test-query")
					return &jsonContent{Value: v}, nil
				})),
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

			var jsonResp jsonContent
			err = json.Unmarshal(b, &jsonResp)
			if !assert.Nil(t, err) {
				return
			}
			if !assert.Equal(t, "abc123", jsonResp.Value) {
				return
			}
		})

		t.Run("if a query param is configured with the QueryParams option and marked required", func(t *testing.T) {
			type jsonContent struct {
				Value string `json:"value"`
			}

			e := NewOperation(
				ProducesJson(HandlerFunc[EmptyRequest, jsonContent](func(ctx context.Context, _ *EmptyRequest) (*jsonContent, error) {
					v := QueryValue(ctx, "test-query")
					return &jsonContent{Value: v}, nil
				})),
				QueryParams(QueryParam{
					Name:     "test-query",
					Required: true,
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

			var jsonResp jsonContent
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
				noopHandler[EmptyRequest, EmptyResponse]{},
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
			type jsonContent struct {
				Value string `json:"value"`
			}

			e := NewOperation(
				ProducesJson(HandlerFunc[EmptyRequest, jsonContent](func(_ context.Context, _ *EmptyRequest) (*jsonContent, error) {
					return &jsonContent{Value: "hello, world"}, nil
				})),
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

			var jsonResp jsonContent
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
				HandlerFunc[EmptyRequest, EmptyResponse](func(_ context.Context, _ *EmptyRequest) (*EmptyResponse, error) {
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

		t.Run("if the underlying Handler returns a nil Response", func(t *testing.T) {
			type jsonContent struct {
				Value string `json:"value"`
			}

			var caughtError error
			e := NewOperation(
				ProducesJson(HandlerFunc[EmptyRequest, jsonContent](func(_ context.Context, _ *EmptyRequest) (*jsonContent, error) {
					return nil, nil
				})),
				OnError(errorHandlerFunc(func(ctx context.Context, w http.ResponseWriter, err error) {
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
			if !assert.ErrorIs(t, ErrNilHandlerResponse, caughtError) {
				return
			}
		})

		t.Run("if the underlying Handler return a nil io.WriterTo", func(t *testing.T) {
			var caughtError error
			e := NewOperation(
				HandlerFunc[EmptyRequest, ReaderContent](func(_ context.Context, _ *EmptyRequest) (*ReaderContent, error) {
					return nil, nil
				}),
				OnError(errorHandlerFunc(func(ctx context.Context, w http.ResponseWriter, err error) {
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
			if !assert.ErrorIs(t, ErrNilHandlerResponse, caughtError) {
				return
			}
		})

		t.Run("if a custom error handler is set", func(t *testing.T) {
			errStatusCode := http.StatusServiceUnavailable
			if !assert.NotEqual(t, DefaultErrorStatusCode, errStatusCode) {
				return
			}

			e := NewOperation(
				HandlerFunc[EmptyRequest, EmptyResponse](func(_ context.Context, _ *EmptyRequest) (*EmptyResponse, error) {
					return nil, errors.New("failed")
				}),
				OnError(errorHandlerFunc(func(ctx context.Context, w http.ResponseWriter, err error) {
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

		t.Run("if a required path param is missing", func(t *testing.T) {
			var caughtError error
			e := NewOperation(
				noopHandler[EmptyRequest, EmptyResponse]{},
				PathParams(
					PathParam{
						Name:     "id",
						Required: true,
					},
				),
				OnError(errorHandlerFunc(func(ctx context.Context, w http.ResponseWriter, err error) {
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

			var merr MissingRequiredPathParamError
			if !assert.ErrorAs(t, caughtError, &merr) {
				return
			}
			if !assert.NotEmpty(t, merr.Error()) {
				return
			}
		})

		t.Run("if a path param does not match its expected pattern", func(t *testing.T) {
			var caughtError error
			e := NewOperation(
				noopHandler[EmptyRequest, EmptyResponse]{},
				PathParams(
					PathParam{
						Name:    "id",
						Pattern: "^[a-zA-Z]*$",
					},
				),
				OnError(errorHandlerFunc(func(ctx context.Context, w http.ResponseWriter, err error) {
					caughtError = err

					w.WriteHeader(DefaultErrorStatusCode)
				})),
			)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.SetPathValue("id", "abc123")

			e.ServeHTTP(w, r)

			resp := w.Result()
			if !assert.Equal(t, DefaultErrorStatusCode, resp.StatusCode) {
				return
			}

			var ierr InvalidPathParamError
			if !assert.ErrorAs(t, caughtError, &ierr) {
				return
			}
			if !assert.NotEmpty(t, ierr.Error()) {
				return
			}
		})

		t.Run("if a required http header is missing", func(t *testing.T) {
			var caughtError error
			e := NewOperation(
				noopHandler[EmptyRequest, EmptyResponse]{},
				Headers(
					Header{
						Name:     "Authorization",
						Required: true,
					},
				),
				OnError(errorHandlerFunc(func(ctx context.Context, w http.ResponseWriter, err error) {
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
				noopHandler[EmptyRequest, EmptyResponse]{},
				Headers(
					Header{
						Name:    "Authorization",
						Pattern: "^[a-zA-Z]*$",
					},
				),
				OnError(errorHandlerFunc(func(ctx context.Context, w http.ResponseWriter, err error) {
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
				noopHandler[EmptyRequest, EmptyResponse]{},
				QueryParams(
					QueryParam{
						Name:     "test",
						Required: true,
					},
				),
				OnError(errorHandlerFunc(func(ctx context.Context, w http.ResponseWriter, err error) {
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
				noopHandler[EmptyRequest, EmptyResponse]{},
				QueryParams(
					QueryParam{
						Name:    "test",
						Pattern: "^[a-zA-Z]*$",
					},
				),
				OnError(errorHandlerFunc(func(ctx context.Context, w http.ResponseWriter, err error) {
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
			type jsonContent struct {
				Value string `json:"value"`
			}

			var caughtError error
			e := NewOperation(
				ConsumesJson(HandlerFunc[jsonContent, EmptyResponse](func(_ context.Context, _ *jsonContent) (*EmptyResponse, error) {
					return &EmptyResponse{}, nil
				})),
				OnError(errorHandlerFunc(func(ctx context.Context, w http.ResponseWriter, err error) {
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
				HandlerFunc[FailReadFrom, EmptyResponse](func(_ context.Context, _ *FailReadFrom) (*EmptyResponse, error) {
					return &EmptyResponse{}, nil
				}),
				OnError(errorHandlerFunc(func(ctx context.Context, w http.ResponseWriter, err error) {
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
			if !assert.Equal(t, errReadFrom, caughtError) {
				return
			}
		})

		t.Run("if the unmarshaled request body is invalid", func(t *testing.T) {
			var caughtError error
			e := NewOperation(
				HandlerFunc[InvalidRequest, EmptyResponse](func(_ context.Context, _ *InvalidRequest) (*EmptyResponse, error) {
					return &EmptyResponse{}, nil
				}),
				OnError(errorHandlerFunc(func(ctx context.Context, w http.ResponseWriter, err error) {
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
	})

	t.Run("will return response header", func(t *testing.T) {
		t.Run("if the response body implements ContentTyper", func(t *testing.T) {
			type jsonContent struct {
				Value string `json:"value"`
			}

			e := NewOperation(
				ProducesJson(HandlerFunc[EmptyRequest, jsonContent](func(_ context.Context, _ *EmptyRequest) (*jsonContent, error) {
					return &jsonContent{Value: "hello, world"}, nil
				})),
			)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/", nil)

			e.ServeHTTP(w, r)

			resp := w.Result()
			if !assert.Equal(t, DefaultStatusCode, resp.StatusCode) {
				return
			}
			if !assert.Equal(t, (&JsonResponse[jsonContent]{}).ContentType(), resp.Header.Get("Content-Type")) {
				return
			}

			b, err := io.ReadAll(resp.Body)
			if !assert.Nil(t, err) {
				return
			}

			var content jsonContent
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
				HandlerFunc[EmptyRequest, EmptyResponse](func(ctx context.Context, _ *EmptyRequest) (*EmptyResponse, error) {
					SetResponseHeader(ctx, "Content-Type", "test-content-type")
					return &EmptyResponse{}, nil
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
