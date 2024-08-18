// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package rest

import (
	"context"
	"net/http"

	"github.com/swaggest/openapi-go"
	"github.com/z5labs/bedrock/rest/endpoint"
)

// Empty
type Empty struct{}

type handleOptions struct {
	statusCode int
	ocOpts     []func(openapi.OperationContext)
	validators []func(*http.Request) error
	injectors  []func(context.Context, *http.Request) context.Context
}

// HandleOption
type HandleOption func(*handleOptions)

func Endpoint[Req, Resp any](e *endpoint.Endpoint[Req, Resp]) Option {
	return func(app *App) {
		oc, err := app.openapi.NewOperationContext(e.Method(), e.Pattern())
		if err != nil {
			panic(err)
		}
		defer app.openapi.AddOperation(oc)

		e.OpenApi(oc)

		app.mux.Handle(e.Pattern(), e)
	}
}
