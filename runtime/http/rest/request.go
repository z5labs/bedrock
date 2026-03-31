// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package rest

// EmptyBody represents the absence of a request body.
type EmptyBody struct{}

// Request carries all decoded handler inputs: parameters and body.
// The type parameter B represents the body type.
// For endpoints without a body, B is EmptyBody.
type Request[B any] struct {
	params paramStore
	body   B
}

// Body returns the decoded request body.
func (r Request[B]) Body() B {
	return r.body
}

// ParamFrom retrieves a typed parameter value from the Request.
// The parameter must have been registered via Param.Read on the endpoint.
func ParamFrom[T any, B any](req Request[B], p Param[T]) T {
	v, ok := req.params.get(p.key)
	if !ok {
		var zero T
		return zero
	}
	return v.(T)
}

// paramStore holds decoded parameter values keyed by their unique identity.
type paramStore struct {
	values map[any]any
}

func (s *paramStore) set(key any, value any) {
	if s.values == nil {
		s.values = make(map[any]any)
	}
	s.values[key] = value
}

func (s *paramStore) get(key any) (any, bool) {
	if s.values == nil {
		return nil, false
	}
	v, ok := s.values[key]
	return v, ok
}
