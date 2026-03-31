// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package rest provides a REST API builder with automatic OpenAPI v3 spec generation.
//
// The package uses an inside-out composition pattern where endpoints are built
// by composing typed readers, writers, and error handlers. Two types enforce
// completeness: Endpoint (incomplete) and Route (complete with all error paths handled).
//
// All API construction is done via explicit function calls rather than struct tags.
// Parameters are declared as typed variables and composed into the endpoint chain,
// then accessed in handlers via ParamFrom. Request bodies and responses are similarly
// declared via ReadJSON/WriteBinary/etc.
//
// The handler signature is func(context.Context, Request[B]) (Resp, error), where B
// is the body type (EmptyBody for no body). All handler inputs (params and body) are
// carried in Request[B], keeping context.Context clean.
//
// # Basic Usage
//
//	var userID = rest.PathParam[string]("id")
//
//	func getUser(ctx context.Context, req rest.Request[rest.EmptyBody]) (User, error) {
//	    id := rest.ParamFrom(req, userID)
//	    return findUser(id)
//	}
//
//	ep := rest.GET("/users/{id}", getUser)
//	ep = userID.Read(ep)
//	ep = rest.WriteJSON[User](200, ep)
//	ep = rest.ErrorJSON[NotFoundError](404, ep)
//	route := rest.CatchAll[GenericError](500, ep)
//
//	handler := rest.Build(
//	    rest.Title("My API"),
//	    rest.Version("1.0.0"),
//	    route.Route(),
//	)
//
// The built handler serves all registered routes plus /openapi.json with the
// auto-generated OpenAPI v3 spec.
//
// # Integration with runtime/http
//
// The Build function returns a bedrock.Builder[http.Handler] that plugs directly
// into the runtime/http package:
//
//	rt := httprt.Build(listener, handler)
//	runner := bedrock.DefaultRunner[httprt.Runtime]()
//	runner.Run(ctx, rt)
package rest
