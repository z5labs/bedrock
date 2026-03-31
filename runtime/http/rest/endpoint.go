// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package rest

import (
	"context"
	"fmt"
	"net/http"

	"github.com/swaggest/openapi-go/openapi3"
)

// Endpoint is an incomplete endpoint definition.
// It cannot be registered with Build until it is completed via CatchAll.
type Endpoint struct {
	method  string
	pattern string

	// handler wraps the user's typed handler into a unified signature.
	handler func(ctx context.Context, params paramStore, body any) (any, error)

	// readers decode params from *http.Request into paramStore.
	readers []func(*http.Request, *paramStore) error

	// bodyDecoder decodes the request body. nil means EmptyBody.
	bodyDecoder func(*http.Request) (any, error)

	// respEncoder writes the success response.
	respEncoder func(http.ResponseWriter, any)
	respStatus  int

	// errEncoders handle specific error types in order.
	errEncoders []errorEncoder

	// specOps register parameters and metadata with the OpenAPI operation.
	specOps []func(*openapi3.Operation)

	// specReqBody registers request body with the reflector.
	specReqBody func(*openapi3.Reflector, *openapi3.Operation) error

	// specResps registers responses with the reflector.
	specResps []func(*openapi3.Reflector, *openapi3.Operation) error

	// metadata
	summary     string
	description string
	tags        []string
	operationID string
	deprecated  bool
}

// errorEncoder matches and encodes a specific error type.
type errorEncoder struct {
	match  func(error) (any, bool)
	encode func(http.ResponseWriter, any)
	status int
	specOp func(*openapi3.Reflector, *openapi3.Operation) error
}

// GET creates an endpoint for HTTP GET requests.
// The handler receives a Request[EmptyBody] since GET requests have no body.
func GET[Resp any](pattern string, handler func(context.Context, Request[EmptyBody]) (Resp, error)) Endpoint {
	return Endpoint{
		method:  http.MethodGet,
		pattern: pattern,
		handler: func(ctx context.Context, params paramStore, body any) (any, error) {
			req := Request[EmptyBody]{params: params, body: EmptyBody{}}
			return handler(ctx, req)
		},
	}
}

// POST creates an endpoint for HTTP POST requests.
// The body type B is inferred from the handler signature.
func POST[B any, Resp any](pattern string, handler func(context.Context, Request[B]) (Resp, error)) Endpoint {
	return Endpoint{
		method:  http.MethodPost,
		pattern: pattern,
		handler: func(ctx context.Context, params paramStore, body any) (any, error) {
			b, ok := body.(B)
			if !ok {
				return nil, fmt.Errorf("internal error: expected body type %T, got %T", *new(B), body)
			}
			req := Request[B]{params: params, body: b}
			return handler(ctx, req)
		},
	}
}

// PUT creates an endpoint for HTTP PUT requests.
// The body type B is inferred from the handler signature.
func PUT[B any, Resp any](pattern string, handler func(context.Context, Request[B]) (Resp, error)) Endpoint {
	return Endpoint{
		method:  http.MethodPut,
		pattern: pattern,
		handler: func(ctx context.Context, params paramStore, body any) (any, error) {
			b, ok := body.(B)
			if !ok {
				return nil, fmt.Errorf("internal error: expected body type %T, got %T", *new(B), body)
			}
			req := Request[B]{params: params, body: b}
			return handler(ctx, req)
		},
	}
}

// PATCH creates an endpoint for HTTP PATCH requests.
// The body type B is inferred from the handler signature.
func PATCH[B any, Resp any](pattern string, handler func(context.Context, Request[B]) (Resp, error)) Endpoint {
	return Endpoint{
		method:  http.MethodPatch,
		pattern: pattern,
		handler: func(ctx context.Context, params paramStore, body any) (any, error) {
			b, ok := body.(B)
			if !ok {
				return nil, fmt.Errorf("internal error: expected body type %T, got %T", *new(B), body)
			}
			req := Request[B]{params: params, body: b}
			return handler(ctx, req)
		},
	}
}

// DELETE creates an endpoint for HTTP DELETE requests.
// The handler receives a Request[EmptyBody] since DELETE requests typically have no body.
func DELETE[Resp any](pattern string, handler func(context.Context, Request[EmptyBody]) (Resp, error)) Endpoint {
	return Endpoint{
		method:  http.MethodDelete,
		pattern: pattern,
		handler: func(ctx context.Context, params paramStore, body any) (any, error) {
			req := Request[EmptyBody]{params: params, body: EmptyBody{}}
			return handler(ctx, req)
		},
	}
}

// Summary sets the OpenAPI summary for the endpoint.
func Summary(s string, ep Endpoint) Endpoint {
	ep.summary = s
	return ep
}

// EndpointDescription sets the OpenAPI description for the endpoint.
func EndpointDescription(s string, ep Endpoint) Endpoint {
	ep.description = s
	return ep
}

// Tags sets the OpenAPI tags for the endpoint.
func Tags(tags []string, ep Endpoint) Endpoint {
	ep.tags = tags
	return ep
}

// OperationID sets the OpenAPI operationId for the endpoint.
func OperationID(id string, ep Endpoint) Endpoint {
	ep.operationID = id
	return ep
}

// MarkDeprecated marks the endpoint as deprecated in the OpenAPI spec.
func MarkDeprecated(ep Endpoint) Endpoint {
	ep.deprecated = true
	return ep
}
