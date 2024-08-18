// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package endpoint

import (
	"context"
	"net/http"
)

func inject(ctx context.Context, r *http.Request, injectors ...func(context.Context, *http.Request) context.Context) context.Context {
	for _, injector := range injectors {
		ctx = injector(ctx, r)
	}
	return ctx
}

type injectKey string

var (
	injectQueryParamsKey = injectKey("injectQueryParamsKey")
	injectHeadersKey     = injectKey("injectHeadersKey")
)

func injectQueryParams(ctx context.Context, r *http.Request) context.Context {
	ctx = context.WithValue(ctx, injectQueryParamsKey, r.URL.Query())
	return ctx
}

func injectHeaders(ctx context.Context, r *http.Request) context.Context {
	ctx = context.WithValue(ctx, injectHeadersKey, r.Header)
	return ctx
}
