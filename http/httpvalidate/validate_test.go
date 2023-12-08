// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package httpvalidate

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHandler_ServeHTTP(t *testing.T) {
	t.Run("will not run base handler", func(t *testing.T) {
		t.Run("if any validator fails", func(t *testing.T) {
			h := Request(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
				}),
				ValidatorFunc(func(w http.ResponseWriter, r *http.Request) bool {
					w.WriteHeader(http.StatusInternalServerError)
					return false
				}),
			)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "http://example.com", nil)

			h.ServeHTTP(w, r)

			responseCode := w.Result().StatusCode
			if !assert.Equal(t, http.StatusInternalServerError, responseCode) {
				return
			}
		})
	})

	t.Run("will run base handler", func(t *testing.T) {
		t.Run("if all validators pass", func(t *testing.T) {
			h := Request(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusAccepted)
				}),
				ValidatorFunc(func(w http.ResponseWriter, r *http.Request) bool {
					return true
				}),
			)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "http://example.com", nil)

			h.ServeHTTP(w, r)

			responseCode := w.Result().StatusCode
			if !assert.Equal(t, http.StatusAccepted, responseCode) {
				return
			}
		})
	})
}

func TestForMethod(t *testing.T) {
	t.Run("will return 405 status code", func(t *testing.T) {
		t.Run("if request method is not in given list", func(t *testing.T) {
			h := Request(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
				}),
				ForMethods(http.MethodGet),
			)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "http://example.com", nil)

			h.ServeHTTP(w, r)

			responseCode := w.Result().StatusCode
			if !assert.Equal(t, http.StatusMethodNotAllowed, responseCode) {
				return
			}
		})
	})
}

func TestMinimumParams(t *testing.T) {
	t.Run("will return a 400 status code", func(t *testing.T) {
		t.Run("if num of params is less than the num of names", func(t *testing.T) {
			h := Request(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
				}),
				MinimumParams("hello"),
			)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "http://example.com", nil)

			h.ServeHTTP(w, r)

			responseCode := w.Result().StatusCode
			if !assert.Equal(t, http.StatusBadRequest, responseCode) {
				return
			}
		})

		t.Run("if one of the names is not of the query params", func(t *testing.T) {
			h := Request(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
				}),
				MinimumParams("hello"),
			)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "http://example.com?good=bye", nil)

			h.ServeHTTP(w, r)

			responseCode := w.Result().StatusCode
			if !assert.Equal(t, http.StatusBadRequest, responseCode) {
				return
			}
		})
	})
}

func TestExactParams(t *testing.T) {
	t.Run("will return a 400 status code", func(t *testing.T) {
		t.Run("if the num of params is less than the num of names", func(t *testing.T) {
			h := Request(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
				}),
				ExactParams("hello"),
			)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "http://example.com", nil)

			h.ServeHTTP(w, r)

			responseCode := w.Result().StatusCode
			if !assert.Equal(t, http.StatusBadRequest, responseCode) {
				return
			}
		})

		t.Run("if the num of params is greater than the num of names", func(t *testing.T) {
			h := Request(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
				}),
				ExactParams("hello"),
			)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "http://example.com?hello=world&good=bye", nil)

			h.ServeHTTP(w, r)

			responseCode := w.Result().StatusCode
			if !assert.Equal(t, http.StatusBadRequest, responseCode) {
				return
			}
		})

		t.Run("if any of the names is not a query param", func(t *testing.T) {
			h := Request(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
				}),
				ExactParams("hello"),
			)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "http://example.com?good=bye", nil)

			h.ServeHTTP(w, r)

			responseCode := w.Result().StatusCode
			if !assert.Equal(t, http.StatusBadRequest, responseCode) {
				return
			}
		})
	})
}
