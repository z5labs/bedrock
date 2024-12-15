// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package key provides types for strongly typed keys in key value pairs.
package key

import (
	"strings"
)

// Keyer is a common interface all value key types must implement.
type Keyer interface {
	Key() string
}

// Chain represents nested keys.
type Chain []Keyer

// Key implements the [Keyer] interface.
func (k Chain) Key() string {
	ss := make([]string, len(k))
	for i := range len(k) {
		ss[i] = k[i].Key()
	}
	return strings.Join(ss, ".")
}

// Name represents a single key. Name can be used other keys.
type Name string

// Key implements the [Keyer] interface.
func (k Name) Key() string {
	return string(k)
}
