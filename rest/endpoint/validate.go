// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package endpoint

import (
	"context"
	"fmt"
	"net/http"
	"regexp"

	"go.opentelemetry.io/otel"
)

func validateRequest(ctx context.Context, r *http.Request, validators ...func(*http.Request) error) error {
	_, span := otel.Tracer("endpoint").Start(ctx, "validateRequest")
	defer span.End()

	for _, validator := range validators {
		err := validator(r)
		if err != nil {
			span.RecordError(err)
			return err
		}
	}
	return nil
}

// InvalidHeaderError
type InvalidHeaderError struct {
	Header string
}

// Error implements the [error] interface.
func (e InvalidHeaderError) Error() string {
	return fmt.Sprintf("received invalid header for endpoint: %s", e.Header)
}

// MissingRequiredHeaderError
type MissingRequiredHeaderError struct {
	Header string
}

// Error implements the [error] interface.
func (e MissingRequiredHeaderError) Error() string {
	return fmt.Sprintf("missing required header for endpoint: %s", e.Header)
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

// InvalidQueryParamError
type InvalidQueryParamError struct {
	Param string
}

// Error implements the [error] interface.
func (e InvalidQueryParamError) Error() string {
	return fmt.Sprintf("received invalid query param for endpoint: %s", e.Param)
}

// MissingRequiredQueryParamError
type MissingRequiredQueryParamError struct {
	Param string
}

// Error implements the [error] interface.
func (e MissingRequiredQueryParamError) Error() string {
	return fmt.Sprintf("missing required query param for endpoint: %s", e.Param)
}

func validateQueryParam(qp QueryParam) func(*http.Request) error {
	var pattern *regexp.Regexp
	if qp.Pattern != "" {
		pattern = regexp.MustCompile(qp.Pattern)
	}

	return func(r *http.Request) error {
		val := r.URL.Query().Get(qp.Name)
		if pattern != nil && !pattern.MatchString(val) {
			return InvalidQueryParamError{Param: qp.Name}
		}
		if !qp.Required {
			return nil
		}
		if val == "" {
			return MissingRequiredQueryParamError{Param: qp.Name}
		}
		return nil
	}
}
