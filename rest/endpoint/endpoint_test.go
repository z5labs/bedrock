// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package endpoint

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

type noopHandler struct{}

func (noopHandler) Handle(_ context.Context, _ Empty) (Empty, error) {
	return Empty{}, nil
}

func TestEndpoint_ServeHTTP(t *testing.T) {
	t.Run("will return non-success http status code", func(t *testing.T) {
		t.Run("if a custom error handler is set", func(t *testing.T) {
			pattern := "/"
			errStatusCode := http.StatusServiceUnavailable
			if !assert.NotEqual(t, defaultErrorStatusCode, errStatusCode) {
				return
			}

			e := Get(
				pattern,
				noopHandler{},
				OnError(errorHandlerFunc(func(ctx context.Context, w http.ResponseWriter, err error) {
					w.WriteHeader(errStatusCode)
				})),
			)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, pattern, nil)

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
			if !assert.Equal(t, defaultErrorStatusCode, resp.StatusCode) {
				return
			}
		})
	})
}
