// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package ptr provides helpers for working with references of values.
package ptr

// Deref returns either the zero value for type T or the
// dereferenced value of t.
func Deref[T any](t *T) T {
	var zero T
	if t == nil {
		return zero
	}
	return *t
}

// Ref returns a reference of the given value.
func Ref[T any](t T) *T {
	return &t
}
