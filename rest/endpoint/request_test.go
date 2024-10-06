// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package endpoint

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

type successValidator struct{}

func (successValidator) Validate() error {
	return nil
}

func TestConsumesJson(t *testing.T) {
	t.Run("will return an error while reading", func(t *testing.T) {
		t.Run("if the request body is not valid json", func(t *testing.T) {
			h := noopHandler[Empty, Empty]{}

			var caughtError error
			op := NewOperation(
				ConsumesJson(h),
				OnError(errorHandlerFunc(func(ctx context.Context, w http.ResponseWriter, err error) {
					caughtError = err

					w.WriteHeader(DefaultErrorStatusCode)
				})),
			)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/", strings.NewReader(``))
			r.Header.Set("Content-Type", JsonRequest[Empty]{}.ContentType())

			op.ServeHTTP(w, r)

			resp := w.Result()
			if !assert.Equal(t, DefaultErrorStatusCode, resp.StatusCode) {
				return
			}

			var serr *json.SyntaxError
			if !assert.ErrorAs(t, caughtError, &serr) {
				return
			}
		})
	})

	t.Run("will return a validation error", func(t *testing.T) {
		t.Run("if the inner type fails to validate", func(t *testing.T) {
			h := noopHandler[InvalidRequest, Empty]{}

			var caughtError error
			op := NewOperation(
				ConsumesJson(h),
				OnError(errorHandlerFunc(func(ctx context.Context, w http.ResponseWriter, err error) {
					caughtError = err

					w.WriteHeader(DefaultErrorStatusCode)
				})),
			)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/", strings.NewReader(`{}`))
			r.Header.Set("Content-Type", JsonRequest[InvalidRequest]{}.ContentType())

			op.ServeHTTP(w, r)

			resp := w.Result()
			if !assert.Equal(t, DefaultErrorStatusCode, resp.StatusCode) {
				return
			}

			if !assert.Equal(t, errInvalidRequest, caughtError) {
				return
			}
		})
	})

	t.Run("will not return a validation error", func(t *testing.T) {
		t.Run("if the inner types successfully validates", func(t *testing.T) {
			h := noopHandler[successValidator, Empty]{}

			var caughtError error
			op := NewOperation(
				ConsumesJson(h),
				OnError(errorHandlerFunc(func(ctx context.Context, w http.ResponseWriter, err error) {
					caughtError = err

					w.WriteHeader(DefaultErrorStatusCode)
				})),
			)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/", strings.NewReader(`{}`))
			r.Header.Set("Content-Type", JsonRequest[successValidator]{}.ContentType())

			op.ServeHTTP(w, r)

			resp := w.Result()
			if !assert.Equal(t, DefaultStatusCode, resp.StatusCode) {
				return
			}
			if !assert.Nil(t, caughtError) {
				return
			}
		})

		t.Run("if the inner type does not implement the Validator interface", func(t *testing.T) {
			type noop struct{}

			h := noopHandler[noop, Empty]{}

			var caughtError error
			op := NewOperation(
				ConsumesJson(h),
				OnError(errorHandlerFunc(func(ctx context.Context, w http.ResponseWriter, err error) {
					caughtError = err

					w.WriteHeader(DefaultErrorStatusCode)
				})),
			)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/", strings.NewReader(`{}`))
			r.Header.Set("Content-Type", JsonRequest[noop]{}.ContentType())

			op.ServeHTTP(w, r)

			resp := w.Result()
			if !assert.Equal(t, DefaultStatusCode, resp.StatusCode) {
				return
			}
			if !assert.Nil(t, caughtError) {
				return
			}
		})
	})
}
