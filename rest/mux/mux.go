// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package mux defines a simple API for all http multiplexers to implement.
package mux

import (
	"fmt"
	"net/http"
	"path"
	"slices"
	"strings"
	"sync"
)

// Method defines an HTTP method expected to be used in a RESTful API.
type Method string

const (
	MethodGet    Method = http.MethodGet
	MethodPut    Method = http.MethodPut
	MethodPost   Method = http.MethodPost
	MethodDelete Method = http.MethodDelete
)

// HttpOption defines a configuration option for [Http].
type HttpOption func(*Http)

// NotFoundHandler will register the given [http.Handler] to handle
// any HTTP requests that do not match any other method-pattern combinations.
func NotFoundHandler(h http.Handler) HttpOption {
	return func(mux *Http) {
		mux.notFound = h
	}
}

// MethodNotAllowedHandler will register the given [http.Handler] to handle
// any HTTP requests whose method does not match the method registered to a pattern.
func MethodNotAllowedHandler(h http.Handler) HttpOption {
	return func(mux *Http) {
		mux.methodNotAllowed = h
	}
}

// Http wraps a [http.ServeMux] and provides some helpers around overriding
// the default "HTTP 404 Not Found" and "HTTP 405 Method Not Allowed" behaviour.
type Http struct {
	mux *http.ServeMux

	initFallbacksOnce sync.Once
	notFound          http.Handler
	methodNotAllowed  http.Handler

	pathMethods map[string][]Method
}

// NewHttp initializes a request multiplexer using the standard [http.ServeMux.]
func NewHttp(opts ...HttpOption) *Http {
	mux := &Http{
		mux:         http.NewServeMux(),
		pathMethods: make(map[string][]Method),
	}
	for _, opt := range opts {
		opt(mux)
	}
	return mux
}

// Handle will register the [http.Handler] for the given method and pattern
// with the underlying [http.ServeMux]. The method and pattern will be formatted
// together as "method pattern" when calling [http.ServeMux.Handle].
func (m *Http) Handle(method Method, pattern string, h http.Handler) {
	m.pathMethods[pattern] = append(m.pathMethods[pattern], method)
	m.mux.Handle(fmt.Sprintf("%s %s", method, pattern), h)

	// {$} is a special case where we only want to exact match the path pattern.
	if strings.HasSuffix(pattern, "{$}") {
		return
	}

	if strings.HasSuffix(pattern, "/") {
		withoutTrailingSlash := pattern[:len(pattern)-1]
		if len(withoutTrailingSlash) == 0 {
			return
		}

		m.pathMethods[withoutTrailingSlash] = append(m.pathMethods[withoutTrailingSlash], method)
		m.mux.Handle(fmt.Sprintf("%s %s", method, withoutTrailingSlash), h)
		return
	}

	// if the end of the path contains the "..." wildcard segment
	// then we can't add a "/" to it since "..." should not be followed
	// by a "/", per the http.ServeMux docs.
	base := path.Base(pattern)
	if strings.Contains(base, "...") {
		return
	}

	withTrailingSlash := pattern + "/"
	m.pathMethods[withTrailingSlash] = append(m.pathMethods[withTrailingSlash], method)
	m.mux.Handle(fmt.Sprintf("%s %s", method, withTrailingSlash), h)
}

// ServeHTTP implements the [http.Handler] interface.
func (m *Http) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.initFallbacksOnce.Do(m.registerFallbackHandlers)

	m.mux.ServeHTTP(w, r)
}

func (m *Http) registerFallbackHandlers() {
	fs := []func(*http.ServeMux){
		registerNotFoundHandler(m.notFound),
		registerMethodNotAllowedHandler(m.methodNotAllowed, m.pathMethods),
	}
	for _, f := range fs {
		f(m.mux)
	}
}

func registerNotFoundHandler(h http.Handler) func(*http.ServeMux) {
	return func(mux *http.ServeMux) {
		if h == nil {
			return
		}
		mux.Handle("/{path...}", h)
	}
}

func registerMethodNotAllowedHandler(h http.Handler, pathMethods map[string][]Method) func(*http.ServeMux) {
	return func(mux *http.ServeMux) {
		if h == nil {
			return
		}
		if len(pathMethods) == 0 {
			return
		}

		// this list is pulled from the OpenAPI v3 Path Item Object documentation.
		supportedMethods := []Method{
			http.MethodGet,
			http.MethodPut,
			http.MethodPost,
			http.MethodDelete,
			http.MethodOptions,
			http.MethodHead,
			http.MethodPatch,
			http.MethodTrace,
		}

		for path, methods := range pathMethods {
			unsupportedMethods := diffSets(supportedMethods, methods)
			for _, method := range unsupportedMethods {
				mux.Handle(fmt.Sprintf("%s %s", method, path), h)
			}
		}
	}
}

func diffSets[T comparable](xs, ys []T) []T {
	zs := make([]T, 0, len(xs))
	for _, x := range xs {
		if slices.Contains(ys, x) {
			continue
		}
		zs = append(zs, x)
	}
	return zs
}
