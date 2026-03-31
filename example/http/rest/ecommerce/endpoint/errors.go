// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package endpoint

// NotFoundError is returned when a requested resource does not exist.
type NotFoundError struct {
	Message string `json:"message"`
}

func (e NotFoundError) Error() string { return e.Message }

// InternalError is the catch-all error type for unexpected failures.
type InternalError struct {
	Message string `json:"message"`
}

func (e InternalError) Error() string { return e.Message }
