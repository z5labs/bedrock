// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package httphealth

import (
	"net/http"

	"github.com/z5labs/bedrock/pkg/health"
)

// NewHandler wraps a health.Metric into an http.Handler.
//
// If m.Healthy returns true, then HTTP status code 200 is
// returned, else, HTTP status code 503 is returned.
func NewHandler(m health.Metric) http.Handler {
	if h, ok := m.(http.Handler); ok {
		return h
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if m.Healthy(ctx) {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	})
}
