// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package config

import (
	"fmt"

	"github.com/z5labs/bedrock/pkg/config/key"
)

// UnknownKeyerError
type UnknownKeyerError struct {
	key key.Keyer
}

// Error implements the error interface.
func (e UnknownKeyerError) Error() string {
	return fmt.Sprintf("config source tried setting config value with unknown key.Keyer: %s", e.key.Key())
}

type inMemoryStore map[string]any

func (m inMemoryStore) Set(k key.Keyer, v any) error {
	return set(m, k, v)
}

func set(m map[string]any, k key.Keyer, v any) error {
	switch x := k.(type) {
	case key.Name:
		m[string(x)] = v
	case key.Chain:
		return setKeyChain(m, x, v)
	default:
		return UnknownKeyerError{key: k}
	}
	return nil
}

// EmptyKeyChainError
type EmptyKeyChainError struct {
	Value any
}

// Error implements the error interface.
func (e EmptyKeyChainError) Error() string {
	return fmt.Sprintf("attempted to set value to an empty key chain: %v", e.Value)
}

// UnexpectedKeyValueTypeError represents the situation when
// a user tries setting a key to a different type than it
// had previously been set to.
type UnexpectedKeyValueTypeError struct {
	Key          string
	ExpectedType string
}

// Error implements the error interface.
func (e UnexpectedKeyValueTypeError) Error() string {
	return fmt.Sprintf("expected key value to be a %s: %s", e.ExpectedType, e.Key)
}

func setKeyChain(m map[string]any, chain key.Chain, v any) error {
	if len(chain) == 0 {
		return EmptyKeyChainError{Value: v}
	}

	root := chain[0]
	if len(chain) == 1 {
		return set(m, root, v)
	}

	old, ok := m[root.Key()]
	if !ok {
		old = make(map[string]any)
		m[root.Key()] = old
	}

	subM, ok := old.(map[string]any)
	if !ok {
		return UnexpectedKeyValueTypeError{
			Key:          root.Key(),
			ExpectedType: "map[string]any",
		}
	}
	return set(subM, chain[1:], v)
}
