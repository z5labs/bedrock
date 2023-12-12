// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package health

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStarted_ServeHTTP(t *testing.T) {
	t.Run("will return 200", func(t *testing.T) {
		t.Run("if it has been started", func(t *testing.T) {
			var s Started
			s.Started()

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)

			s.ServeHTTP(w, req)
			if !assert.Equal(t, http.StatusOK, w.Result().StatusCode) {
				return
			}
		})
	})

	t.Run("will return 503", func(t *testing.T) {
		t.Run("if it is the zero value", func(t *testing.T) {
			var s Started

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)

			s.ServeHTTP(w, req)
			if !assert.Equal(t, http.StatusServiceUnavailable, w.Result().StatusCode) {
				return
			}
		})
	})
}

func TestReadinessServeHTTP(t *testing.T) {
	t.Run("will return 200", func(t *testing.T) {
		t.Run("if it has been marked ready", func(t *testing.T) {
			var s Readiness
			s.Ready()

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)

			s.ServeHTTP(w, req)
			if !assert.Equal(t, http.StatusOK, w.Result().StatusCode) {
				return
			}
		})
	})

	t.Run("will return 503", func(t *testing.T) {
		t.Run("if it is the zero value", func(t *testing.T) {
			var s Readiness

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)

			s.ServeHTTP(w, req)
			if !assert.Equal(t, http.StatusServiceUnavailable, w.Result().StatusCode) {
				return
			}
		})

		t.Run("if it has been marked not ready", func(t *testing.T) {
			var s Readiness
			s.NotReady()

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)

			s.ServeHTTP(w, req)
			if !assert.Equal(t, http.StatusServiceUnavailable, w.Result().StatusCode) {
				return
			}
		})
	})
}

func TestLiveness_ServeHTTP(t *testing.T) {
	t.Run("will return 200", func(t *testing.T) {
		t.Run("if it has been marked alive", func(t *testing.T) {
			var s Liveness
			s.Alive()

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)

			s.ServeHTTP(w, req)
			if !assert.Equal(t, http.StatusOK, w.Result().StatusCode) {
				return
			}
		})
	})

	t.Run("will return 503", func(t *testing.T) {
		t.Run("if it is the zero value", func(t *testing.T) {
			var s Liveness

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)

			s.ServeHTTP(w, req)
			if !assert.Equal(t, http.StatusServiceUnavailable, w.Result().StatusCode) {
				return
			}
		})

		t.Run("if it has been marked dead", func(t *testing.T) {
			var s Liveness
			s.Dead()

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)

			s.ServeHTTP(w, req)
			if !assert.Equal(t, http.StatusServiceUnavailable, w.Result().StatusCode) {
				return
			}
		})
	})
}
