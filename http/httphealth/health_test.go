// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package httphealth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/z5labs/bedrock/pkg/health"

	"github.com/stretchr/testify/assert"
)

type healthMetricFunc func(context.Context) bool

func (f healthMetricFunc) Healthy(ctx context.Context) bool {
	return f(ctx)
}

type healthMetricHandler struct {
	health.Metric
	http.Handler
}

func TestNewHandler(t *testing.T) {
	t.Run("will return health.Metric", func(t *testing.T) {
		t.Run("if it implements http.Handler", func(t *testing.T) {
			m := healthMetricHandler{
				Metric: healthMetricFunc(func(ctx context.Context) bool {
					return true
				}),
				Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusAccepted)
				}),
			}

			h := NewHandler(m)
			if !assert.IsType(t, m, h) {
				return
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)

			h.ServeHTTP(w, req)
			if !assert.Equal(t, http.StatusAccepted, w.Result().StatusCode) {
				return
			}
		})
	})

	t.Run("will return 200", func(t *testing.T) {
		t.Run("if health.Metric.Healthy returns true", func(t *testing.T) {
			m := healthMetricFunc(func(ctx context.Context) bool {
				return true
			})

			h := NewHandler(m)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)

			h.ServeHTTP(w, req)
			if !assert.Equal(t, http.StatusOK, w.Result().StatusCode) {
				return
			}
		})
	})

	t.Run("will return 503", func(t *testing.T) {
		t.Run("if health.Metric.Healthy returns false", func(t *testing.T) {
			m := healthMetricFunc(func(ctx context.Context) bool {
				return false
			})

			h := NewHandler(m)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)

			h.ServeHTTP(w, req)
			if !assert.Equal(t, http.StatusServiceUnavailable, w.Result().StatusCode) {
				return
			}
		})
	})
}
