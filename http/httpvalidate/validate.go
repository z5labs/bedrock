// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package httpvalidate

import (
	"net/http"
)

// Validator represents an http.Request validator.
type Validator interface {
	Validate(http.ResponseWriter, *http.Request) bool
}

// ValidatorFunc implements Validator for funcs.
type ValidatorFunc func(http.ResponseWriter, *http.Request) bool

// Validate implements the Validator interface.
func (f ValidatorFunc) Validate(w http.ResponseWriter, r *http.Request) bool {
	return f(w, r)
}

// Handler is an http.Handler which applies request validators
// before passing the request to a wrapped http.Handler.
type Handler struct {
	validators []Validator
	base       http.Handler
}

// Request allows you to wrap a given http.Handler with request validators.
func Request(h http.Handler, validators ...Validator) *Handler {
	return &Handler{
		validators: validators,
		base:       h,
	}
}

// ServeHTTP implements the http.Handler interface.
func (h *Handler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	for _, validator := range h.validators {
		valid := validator.Validate(w, req)
		if !valid {
			return
		}
	}
	h.base.ServeHTTP(w, req)
}

// ForMethods will validate the incoming requests' method is one of the given.
func ForMethods(methods ...string) Validator {
	return ValidatorFunc(func(w http.ResponseWriter, r *http.Request) bool {
		for _, method := range methods {
			if method == r.Method {
				return true
			}
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
		return false
	})
}

// MinimumParams validates that the incoming HTTP request
// has, at minimum, the query parameters given by the argument, names.
func MinimumParams(names ...string) Validator {
	minNumOfParams := len(names)
	return ValidatorFunc(func(w http.ResponseWriter, r *http.Request) bool {
		params := r.URL.Query()
		if len(params) < minNumOfParams {
			w.WriteHeader(http.StatusBadRequest)
			return false
		}
		for _, name := range names {
			if !params.Has(name) {
				w.WriteHeader(http.StatusBadRequest)
				return false
			}
		}
		return true
	})
}

// ExactParams validates that the incoming HTTP request
// has the exact query parameters given by the argument, names.
func ExactParams(names ...string) Validator {
	exactNumOfParams := len(names)
	return ValidatorFunc(func(w http.ResponseWriter, r *http.Request) bool {
		params := r.URL.Query()
		if len(params) != exactNumOfParams {
			w.WriteHeader(http.StatusBadRequest)
			return false
		}
		for _, name := range names {
			if !params.Has(name) {
				w.WriteHeader(http.StatusBadRequest)
				return false
			}
		}
		return true
	})
}

// MinProto validates that the incoming HTTP request
// protocol version is greater than or equal to the given
// major, minor version.
func MinProto(major, minor int) Validator {
	return ValidatorFunc(func(w http.ResponseWriter, r *http.Request) bool {
		if r.ProtoAtLeast(major, minor) {
			return true
		}
		w.WriteHeader(http.StatusHTTPVersionNotSupported)
		return false
	})
}
