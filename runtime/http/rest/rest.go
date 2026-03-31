// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package rest

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/swaggest/openapi-go/openapi3"
	"github.com/z5labs/bedrock"
)

// Option configures the API builder.
// Both API-level settings and Routes implement Option.
type Option func(*api)

type api struct {
	title       string
	version     string
	description string
	specPath    string
	routes      []Route
}

// Title sets the API title in the OpenAPI spec.
func Title(t string) Option {
	return func(a *api) {
		a.title = t
	}
}

// Version sets the API version in the OpenAPI spec.
func Version(v string) Option {
	return func(a *api) {
		a.version = v
	}
}

// APIDescription sets the API description in the OpenAPI spec.
func APIDescription(d string) Option {
	return func(a *api) {
		a.description = d
	}
}

// SpecPath sets the path for the OpenAPI JSON spec endpoint.
// Defaults to "/openapi.json".
func SpecPath(path string) Option {
	return func(a *api) {
		a.specPath = path
	}
}

// Route returns an Option that registers this route with the API.
// This allows passing Route values directly to Build.
func (r Route) Route() Option {
	return func(a *api) {
		a.routes = append(a.routes, r)
	}
}

// Build constructs a bedrock.Builder[http.Handler] from the given options.
// The returned handler serves all registered routes and an OpenAPI v3 spec
// endpoint at the configured path (default: /openapi.json).
func Build(opts ...Option) bedrock.Builder[http.Handler] {
	return bedrock.BuilderFunc[http.Handler](func(ctx context.Context) (http.Handler, error) {
		a := &api{
			title:    "API",
			version:  "0.0.0",
			specPath: "/openapi.json",
		}

		// Separate route options from config options.
		// Routes passed directly as Options use Route().
		for _, opt := range opts {
			opt(a)
		}

		// Build OpenAPI spec.
		reflector := openapi3.NewReflector()
		reflector.Spec.Info.
			WithTitle(a.title).
			WithVersion(a.version)
		if a.description != "" {
			reflector.Spec.Info.WithDescription(a.description)
		}

		router := chi.NewRouter()

		for _, route := range a.routes {
			ep := route.endpoint

			// Build the OpenAPI operation for this route.
			op := &openapi3.Operation{}

			// Apply metadata.
			if ep.summary != "" {
				op.WithSummary(ep.summary)
			}
			if ep.description != "" {
				op.WithDescription(ep.description)
			}
			if len(ep.tags) > 0 {
				op.WithTags(ep.tags...)
			}
			if ep.operationID != "" {
				op.WithID(ep.operationID)
			}
			if ep.deprecated {
				t := true
				op.WithDeprecated(t)
			}

			// Apply spec operations (parameters).
			for _, specOp := range ep.specOps {
				specOp(op)
			}

			// Apply request body spec.
			if ep.specReqBody != nil {
				if err := ep.specReqBody(reflector, op); err != nil {
					return nil, err
				}
			}

			// Apply response specs.
			for _, specResp := range ep.specResps {
				if err := specResp(reflector, op); err != nil {
					return nil, err
				}
			}

			// Apply error response specs.
			for _, enc := range ep.errEncoders {
				if err := enc.specOp(reflector, op); err != nil {
					return nil, err
				}
			}

			// Apply catch-all error response spec.
			if err := route.catchAll.specOp(reflector, op); err != nil {
				return nil, err
			}

			// Register the operation in the spec.
			if err := registerOperation(reflector, ep.method, ep.pattern, op); err != nil {
				return nil, err
			}

			// Build the HTTP handler for this route.
			handler := buildRouteHandler(route)
			router.Method(ep.method, ep.pattern, handler)
		}

		// Marshal the spec once at build time.
		specJSON, err := reflector.Spec.MarshalJSON()
		if err != nil {
			return nil, err
		}

		// Register the spec endpoint.
		router.Get(a.specPath, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write(specJSON) //nolint:errcheck
		})

		return router, nil
	})
}

// registerOperation adds an operation to the reflector's spec at the given method and path.
func registerOperation(reflector *openapi3.Reflector, method, pattern string, op *openapi3.Operation) error {
	return reflector.Spec.AddOperation(method, pattern, *op)
}

// buildRouteHandler creates an http.Handler for a complete Route.
func buildRouteHandler(route Route) http.Handler {
	ep := route.endpoint
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Step 1: Decode parameters.
		var store paramStore
		for _, reader := range ep.readers {
			if err := reader(r, &store); err != nil {
				writeValidationError(w, err)
				return
			}
		}

		// Step 2: Decode body.
		var body any
		if ep.bodyDecoder != nil {
			var err error
			body, err = ep.bodyDecoder(r)
			if err != nil {
				writeValidationError(w, err)
				return
			}
		} else {
			body = EmptyBody{}
		}

		// Step 3: Call handler.
		resp, err := ep.handler(r.Context(), store, body)
		if err != nil {
			// Step 4: Try error encoders in order.
			for _, enc := range ep.errEncoders {
				if matched, ok := enc.match(err); ok {
					enc.encode(w, matched)
					return
				}
			}
			// Step 5: Catch-all.
			if matched, ok := route.catchAll.match(err); ok {
				route.catchAll.encode(w, matched)
				return
			}
			// Should never reach here since catch-all always matches.
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Step 6: Encode success response.
		if ep.respEncoder != nil {
			ep.respEncoder(w, resp)
		}
	})
}

func writeValidationError(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	// Write a JSON error response for validation failures.
	// Use json.Marshal to properly escape the error message.
	if ve, ok := err.(*ValidationError); ok {
		json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck
			"param": ve.Param,
			"error": ve.Message,
		})
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"error": err.Error()}) //nolint:errcheck
}
