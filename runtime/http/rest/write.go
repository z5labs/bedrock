// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package rest

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/swaggest/openapi-go/openapi3"
)

// WriteJSON composes JSON response encoding onto the endpoint.
// The status code and response type are registered in the OpenAPI spec.
func WriteJSON[Resp any](status int, ep Endpoint) Endpoint {
	ep.respStatus = status
	ep.respEncoder = func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(v) //nolint:errcheck
	}
	ep.specResps = append(ep.specResps, makeRespSpecOp[Resp](status))
	return ep
}

// WriteBinary composes binary response encoding onto the endpoint.
// The handler must return an io.Reader as the response.
func WriteBinary(status int, contentType string, ep Endpoint) Endpoint {
	ep.respStatus = status
	ep.respEncoder = func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(status)
		if r, ok := v.(io.Reader); ok {
			io.Copy(w, r) //nolint:errcheck
			if c, ok := r.(io.Closer); ok {
				c.Close() //nolint:errcheck
			}
		}
	}
	ep.specResps = append(ep.specResps, func(_ *openapi3.Reflector, op *openapi3.Operation) error {
		resp := openapi3.Response{
			Content: map[string]openapi3.MediaType{
				contentType: {
					Schema: &openapi3.SchemaOrRef{
						Schema: &openapi3.Schema{
							Type:   ptrSchemaType(openapi3.SchemaTypeString),
							Format: ptrString("binary"),
						},
					},
				},
			},
		}
		resp.WithDescription("Binary response")
		op.Responses.WithMapOfResponseOrRefValuesItem(strconv.Itoa(status), openapi3.ResponseOrRef{Response: &resp})
		return nil
	})
	return ep
}

// ErrorJSON composes a typed error response onto the endpoint.
// At runtime, if the handler returns an error matching type E (via errors.As),
// it is encoded as JSON with the given status code.
// The error type and status code are registered in the OpenAPI spec.
func ErrorJSON[E error](status int, ep Endpoint) Endpoint {
	ep.errEncoders = append(ep.errEncoders, errorEncoder{
		status: status,
		match: func(err error) (any, bool) {
			var target E
			if errors.As(err, &target) {
				return target, true
			}
			return nil, false
		},
		encode: func(w http.ResponseWriter, v any) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			json.NewEncoder(w).Encode(v) //nolint:errcheck
		},
		specOp: makeRespSpecOp[E](status),
	})
	return ep
}

// Route is a complete endpoint definition with all error paths handled.
// Only Route can be passed to Build.
type Route struct {
	endpoint Endpoint
	catchAll errorEncoder
}

// CatchAll completes the endpoint by adding a catch-all error handler.
// This is required — an Endpoint cannot be registered without it.
// Any error that doesn't match a specific ErrorJSON handler is caught here
// and wrapped as type E for consistent response formatting.
func CatchAll[E error](status int, wrapError func(error) E, ep Endpoint) Route {
	return Route{
		endpoint: ep,
		catchAll: errorEncoder{
			status: status,
			match: func(err error) (any, bool) {
				var target E
				if errors.As(err, &target) {
					return target, true
				}
				// Wrap the error to ensure consistent response type.
				return wrapError(err), true
			},
			encode: func(w http.ResponseWriter, v any) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				json.NewEncoder(w).Encode(v) //nolint:errcheck
			},
			specOp: makeRespSpecOp[E](status),
		},
	}
}

// makeRespSpecOp creates a spec operation that registers a response type
// at a given status code using the reflector's schema generation.
func makeRespSpecOp[T any](status int) func(*openapi3.Reflector, *openapi3.Operation) error {
	return func(refl *openapi3.Reflector, op *openapi3.Operation) error {
		return refl.SetJSONResponse(op, new(T), status) //nolint:staticcheck
	}
}
