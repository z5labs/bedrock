// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package endpoint

import (
	"context"
	"net/http"
	"net/url"
)

type injector func(context.Context, http.ResponseWriter, *http.Request) context.Context

func inject(ctx context.Context, w http.ResponseWriter, r *http.Request, injectors ...injector) context.Context {
	for _, injector := range injectors {
		ctx = injector(ctx, w, r)
	}
	return ctx
}

type injectKey string

var (
	injectQueryParamsKey     = injectKey("injectQueryParamsKey")
	injectHeadersKey         = injectKey("injectHeadersKey")
	injectResponseHeadersKey = injectKey("injectResponseHeadersKey")
)

type injectPathParamKey string

// PathValue returns the value for a path parameter of the given name.
// The empty string will be returned either if the path param is not set
// or not found.
func PathValue(ctx context.Context, name string) string {
	s, _ := ctx.Value(injectPathParamKey(name)).(string)
	return s
}

func injectPathParam(name string) injector {
	return func(ctx context.Context, w http.ResponseWriter, r *http.Request) context.Context {
		ctx = context.WithValue(ctx, injectPathParamKey(name), r.PathValue(name))
		return ctx
	}
}

// QueryValue returns the value for a query parameter of the given name.
// The empty string will be returned either if the query param is not set
// or not found.
func QueryValue(ctx context.Context, name string) string {
	vals, ok := ctx.Value(injectQueryParamsKey).(url.Values)
	if !ok {
		return ""
	}

	return vals.Get(name)
}

func injectQueryParams(ctx context.Context, w http.ResponseWriter, r *http.Request) context.Context {
	ctx = context.WithValue(ctx, injectQueryParamsKey, r.URL.Query())
	return ctx
}

// HeaderValue returns the value for a header of the given name.
// The empty string will be returned either if the header is not set
// or not found.
func HeaderValue(ctx context.Context, name string) string {
	headers, ok := ctx.Value(injectHeadersKey).(http.Header)
	if !ok {
		return ""
	}
	return headers.Get(name)
}

func injectHeaders(ctx context.Context, w http.ResponseWriter, r *http.Request) context.Context {
	ctx = context.WithValue(ctx, injectHeadersKey, r.Header)
	return ctx
}

// SetResponseHeader allows you set a custom response header.
func SetResponseHeader(ctx context.Context, key, value string) {
	headers, ok := ctx.Value(injectResponseHeadersKey).(http.Header)
	if !ok || headers == nil {
		return
	}

	headers.Set(key, value)
}

func injectResponseHeaders(ctx context.Context, w http.ResponseWriter, r *http.Request) context.Context {
	ctx = context.WithValue(ctx, injectResponseHeadersKey, w.Header())
	return ctx
}
