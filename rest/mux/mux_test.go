// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package mux

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
)

type statusCodeHandler int

func (h statusCodeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(int(h))
}

func TestNotFoundHandler(t *testing.T) {
	testCases := []struct {
		Name            string
		RegisterPattern string
		RequestPath     string
		NotFound        bool
	}{
		{
			Name:        "should match not found if no other endpoints are registered and '/' is requested",
			RequestPath: "/",
			NotFound:    true,
		},
		{
			Name:        "should match not found if no other endpoints are registered and a sub path is requested",
			RequestPath: "/hello",
			NotFound:    true,
		},
		{
			Name:            "should match not found if other endpoints are registered and '/' is requested",
			RegisterPattern: "/hello",
			RequestPath:     "/",
			NotFound:        true,
		},
		{
			Name:            "should match not found if other endpoints are registered and unknown sub-path is requested",
			RegisterPattern: "/hello",
			RequestPath:     "/bye",
			NotFound:        true,
		},
		{
			Name:            "should match not found if '/{$}' is registered and a sub-path is requested",
			RegisterPattern: "/{$}",
			RequestPath:     "/bye",
			NotFound:        true,
		},
		{
			Name:            "should not match not found if endpoint pattern is requested",
			RegisterPattern: "/hello",
			RequestPath:     "/hello",
			NotFound:        false,
		},
		{
			Name:            "should not match not found if '/{$}' is registered and '/' requested",
			RegisterPattern: "/{$}",
			RequestPath:     "/",
			NotFound:        false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.Name, func(t *testing.T) {
			notFoundHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)

				enc := json.NewEncoder(w)
				enc.Encode(map[string]any{"hello": "world"})
			})

			mux := NewHttp(
				NotFoundHandler(notFoundHandler),
			)

			if testCase.RegisterPattern != "" {
				mux.Handle(MethodGet, testCase.RegisterPattern, statusCodeHandler(http.StatusOK))
			}

			w := httptest.NewRecorder()
			r := httptest.NewRequest(
				http.MethodGet,
				fmt.Sprintf("http://%s", path.Join("example.com", testCase.RequestPath)),
				nil,
			)

			mux.ServeHTTP(w, r)

			resp := w.Result()
			if !assert.NotNil(t, resp) {
				return
			}
			if !testCase.NotFound {
				assert.Equal(t, http.StatusOK, resp.StatusCode)
				return
			}
			if !assert.Equal(t, http.StatusNotFound, resp.StatusCode) {
				return
			}
			defer resp.Body.Close()

			b, err := io.ReadAll(resp.Body)
			if !assert.Nil(t, err) {
				return
			}

			var m map[string]any
			err = json.Unmarshal(b, &m)
			if !assert.Nil(t, err) {
				return
			}
			if !assert.Contains(t, m, "hello") {
				return
			}
			if !assert.Equal(t, "world", m["hello"]) {
				return
			}
		})
	}
}

func TestMethodNotAllowedHandler(t *testing.T) {
	testCases := []struct {
		Name             string
		RegisterPatterns map[Method]string
		Method           Method
		RequestPath      string
		MethodNotAllowed bool
	}{
		{
			Name: "should return success response when correct method is used",
			RegisterPatterns: map[Method]string{
				http.MethodGet: "/",
			},
			Method:           MethodGet,
			RequestPath:      "/",
			MethodNotAllowed: false,
		},
		{
			Name: "should return success response when more than one method is registered for same path",
			RegisterPatterns: map[Method]string{
				http.MethodGet:  "/",
				http.MethodPost: "/",
			},
			Method:           MethodGet,
			RequestPath:      "/",
			MethodNotAllowed: false,
		},
		{
			Name: "should return method not allowed response when incorrect method is used",
			RegisterPatterns: map[Method]string{
				http.MethodGet: "/",
			},
			Method:           MethodPost,
			RequestPath:      "/",
			MethodNotAllowed: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.Name, func(t *testing.T) {
			methodNotAllowedHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusMethodNotAllowed)

				enc := json.NewEncoder(w)
				enc.Encode(map[string]any{"hello": "world"})
			})

			mux := NewHttp(
				MethodNotAllowedHandler(methodNotAllowedHandler),
			)

			for method, pattern := range testCase.RegisterPatterns {
				mux.Handle(method, pattern, statusCodeHandler(http.StatusOK))
			}

			w := httptest.NewRecorder()
			r := httptest.NewRequest(
				string(testCase.Method),
				fmt.Sprintf("http://%s", path.Join("example.com", testCase.RequestPath)),
				nil,
			)

			mux.ServeHTTP(w, r)

			resp := w.Result()
			if !assert.NotNil(t, resp) {
				return
			}
			if !testCase.MethodNotAllowed {
				assert.Equal(t, http.StatusOK, resp.StatusCode)
				return
			}
			if !assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode) {
				return
			}
			defer resp.Body.Close()

			b, err := io.ReadAll(resp.Body)
			if !assert.Nil(t, err) {
				return
			}

			var m map[string]any
			err = json.Unmarshal(b, &m)
			if !assert.Nil(t, err) {
				return
			}
			if !assert.Contains(t, m, "hello") {
				return
			}
			if !assert.Equal(t, "world", m["hello"]) {
				return
			}
		})
	}
}
