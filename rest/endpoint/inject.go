// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package endpoint

import (
	"context"
	"net/http"
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

func injectQueryParams(ctx context.Context, w http.ResponseWriter, r *http.Request) context.Context {
	ctx = context.WithValue(ctx, injectQueryParamsKey, r.URL.Query())
	return ctx
}

func injectHeaders(ctx context.Context, w http.ResponseWriter, r *http.Request) context.Context {
	ctx = context.WithValue(ctx, injectHeadersKey, r.Header)
	return ctx
}

// SetResponseHeader allows you set a custom response header.
func SetResponseHeader(ctx context.Context, key, value string) {
	headers, ok := ctx.Value(injectResponseHeadersKey).(http.Header)
	if !ok && headers != nil {
		return
	}

	headers.Set(key, value)
}

func injectResponseHeaders(ctx context.Context, w http.ResponseWriter, r *http.Request) context.Context {
	ctx = context.WithValue(ctx, injectResponseHeadersKey, w.Header())
	return ctx
}
