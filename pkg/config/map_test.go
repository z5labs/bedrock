// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package config

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/z5labs/bedrock/pkg/config/key"

	"github.com/stretchr/testify/assert"
)

type storeFunc func(key.Keyer, any) error

func (f storeFunc) Set(k key.Keyer, v any) error {
	return f(k, v)
}

func TestMap_Apply(t *testing.T) {
	t.Run("will properly construct key.Chain for", func(t *testing.T) {
		testCases := []struct {
			Name   string
			M      Map
			Chains []key.Chain
		}{
			{
				Name: "single top level key",
				M: Map{
					"hello": "world",
				},
				Chains: []key.Chain{
					{key.Name("hello")},
				},
			},
			{
				Name: "multiple top level keys",
				M: Map{
					"hello": "world",
					"one":   1,
				},
				Chains: []key.Chain{
					{key.Name("hello")},
					{key.Name("one")},
				},
			},
			{
				Name: "single nested key",
				M: Map{
					"hello": map[string]any{
						"good": "bye",
					},
				},
				Chains: []key.Chain{
					{key.Name("hello"), key.Name("good")},
				},
			},
			{
				Name: "multiple nested keys",
				M: Map{
					"hello": map[string]any{
						"good":  "bye",
						"alice": "hi bob",
					},
				},
				Chains: []key.Chain{
					{key.Name("hello"), key.Name("alice")},
					{key.Name("hello"), key.Name("good")},
				},
			},
			{
				Name: "keys within slices should not be chained",
				M: Map{
					"hello": []map[string]any{
						{
							"alice": "im bob",
						},
						{
							"bob": "im alice",
						},
					},
				},
				Chains: []key.Chain{
					{key.Name("hello")},
				},
			},
		}

		for _, testCase := range testCases {
			t.Run(testCase.Name, func(t *testing.T) {
				chains := make([]key.Chain, 0, len(testCase.Chains))
				store := storeFunc(func(k key.Keyer, a any) error {
					kc, ok := k.(key.Chain)
					if !ok {
						return errors.New("should only set using a key chain")
					}
					chains = append(chains, kc)
					return nil
				})

				err := testCase.M.Apply(store)
				if !assert.Nil(t, err) {
					return
				}

				// key chains come from a map which is unordered thus
				// we need to sort the chains before comparing slices
				slices.SortFunc(chains, func(a, b key.Chain) int {
					return strings.Compare(a.Key(), b.Key())
				})

				if !assert.Equal(t, testCase.Chains, chains) {
					return
				}
			})
		}
	})

	t.Run("will return an error", func(t *testing.T) {
		t.Run("if the given Store fails to set key", func(t *testing.T) {
			setErr := errors.New("failed to set key")
			store := storeFunc(func(k key.Keyer, a any) error {
				return setErr
			})

			m := Map{"hello": "world"}
			err := m.Apply(store)
			if !assert.ErrorIs(t, err, setErr) {
				return
			}
		})

		t.Run("if the given Store fails to set a nested key", func(t *testing.T) {
			setErr := errors.New("failed to set key")
			store := storeFunc(func(k key.Keyer, a any) error {
				return setErr
			})

			m := Map{
				"hello": map[string]any{
					"bob": "how are you?",
				},
			}
			err := m.Apply(store)
			if !assert.ErrorIs(t, err, setErr) {
				return
			}
		})
	})
}
