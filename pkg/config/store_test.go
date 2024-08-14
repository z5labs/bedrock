// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package config

import (
	"testing"

	"github.com/z5labs/bedrock/pkg/config/key"

	"github.com/stretchr/testify/assert"
)

type myKeyer string

func (myKeyer) Key() string {
	return "my key"
}

func TestInMemoryStore_Set(t *testing.T) {
	t.Run("will return an error", func(t *testing.T) {
		t.Run("if an known key.Keyer is used", func(t *testing.T) {
			store := make(inMemoryStore)
			err := store.Set(myKeyer("hello"), "world")

			var ierr UnknownKeyerError
			if !assert.ErrorAs(t, err, &ierr) {
				return
			}
			if !assert.NotEmpty(t, ierr.Error()) {
				return
			}
		})

		t.Run("if an empty key.Chain is used", func(t *testing.T) {
			store := make(inMemoryStore)
			err := store.Set(key.Chain{}, "world")

			var ierr EmptyKeyChainError
			if !assert.ErrorAs(t, err, &ierr) {
				return
			}
			if !assert.NotEmpty(t, ierr.Error()) {
				return
			}
		})

		t.Run("if the value type is attempted to be changed while overriding an existing key", func(t *testing.T) {
			store := make(inMemoryStore)
			err := store.Set(key.Name("hello"), "world")
			if !assert.Nil(t, err) {
				return
			}

			err = store.Set(key.Chain{key.Name("hello"), key.Name("bob")}, "world")

			var ierr UnexpectedKeyValueTypeError
			if !assert.ErrorAs(t, err, &ierr) {
				return
			}
			if !assert.NotEmpty(t, ierr.Error()) {
				return
			}
		})
	})
}
