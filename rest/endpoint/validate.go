// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package endpoint

import (
	"fmt"
	"net/http"
	"regexp"
)

func validateRequest(r *http.Request, validators ...func(*http.Request) error) error {
	for _, validator := range validators {
		err := validator(r)
		if err != nil {
			return err
		}
	}
	return nil
}

// InvalidMethodError represents when a request was sent to an endpoint
// for the incorrect method.
type InvalidMethodError struct {
	Method string
}

// Error implements [error] interface.
func (e InvalidMethodError) Error() string {
	return fmt.Sprintf("received invalid method for endpoint: %s", e.Method)
}

// ServeHTTP implements the [http.Handler] interface.
func (InvalidMethodError) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusMethodNotAllowed)
}

func validateMethod(method string) func(*http.Request) error {
	return func(r *http.Request) error {
		if r.Method == method {
			return nil
		}
		return InvalidMethodError{Method: r.Method}
	}
}

// InvalidHeaderError
type InvalidHeaderError struct {
	Header string
}

// Error implements the [error] interface.
func (e InvalidHeaderError) Error() string {
	return fmt.Sprintf("received invalid header for endpoint: %s", e.Header)
}

// ServeHTTP implements the [http.Handler] interface.
func (InvalidHeaderError) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusBadRequest)
}

// MissingRequiredHeaderError
type MissingRequiredHeaderError struct {
	Header string
}

// Error implements the [error] interface.
func (e MissingRequiredHeaderError) Error() string {
	return fmt.Sprintf("missing required header for endpoint: %s", e.Header)
}

// ServeHTTP implements the [http.Handler] interface.
func (MissingRequiredHeaderError) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusBadRequest)
}

func validateHeader(h Header) func(*http.Request) error {
	var pattern *regexp.Regexp
	if h.Pattern != "" {
		pattern = regexp.MustCompile(h.Pattern)
	}

	return func(r *http.Request) error {
		val := r.Header.Get(h.Name)
		if pattern != nil && !pattern.MatchString(val) {
			return InvalidHeaderError{Header: h.Name}
		}
		if !h.Required {
			return nil
		}
		if val == "" {
			return MissingRequiredHeaderError{Header: h.Name}
		}
		return nil
	}
}

func validateQueryParam(qp QueryParam) func(*http.Request) error {
	return func(r *http.Request) error {
		// TODO
		return nil
	}
}
