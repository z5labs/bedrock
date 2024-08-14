// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package config

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/z5labs/bedrock/pkg/config/key"
)

type readFunc func([]byte) (int, error)

func (f readFunc) Read(b []byte) (int, error) {
	return f(b)
}

func TestYaml_Apply(t *testing.T) {
	t.Run("will return an error", func(t *testing.T) {
		t.Run("if the underlying io.Reader fails", func(t *testing.T) {
			readErr := errors.New("failed to read")
			r := readFunc(func(b []byte) (int, error) {
				return 0, readErr
			})

			store := storeFunc(func(k key.Keyer, a any) error {
				return nil
			})

			src := FromYaml(r)
			err := src.Apply(store)
			if !assert.ErrorIs(t, err, readErr) {
				return
			}
		})

		t.Run("if the io.Reader contains invalid YAML", func(t *testing.T) {
			r := strings.NewReader(`hello`)

			store := storeFunc(func(k key.Keyer, a any) error {
				return nil
			})

			src := FromYaml(r)
			err := src.Apply(store)

			var ierr InvalidYamlError
			if !assert.ErrorAs(t, err, &ierr) {
				return
			}
			if !assert.NotEmpty(t, ierr.Error()) {
				return
			}
			if !assert.NotNil(t, ierr.Unwrap()) {
				return
			}
		})

		t.Run("if the underlying store fails to set a key", func(t *testing.T) {
			r := strings.NewReader(`hello: world`)

			storeErr := errors.New("failed to set key")
			store := storeFunc(func(k key.Keyer, a any) error {
				return storeErr
			})

			src := FromYaml(r)
			err := src.Apply(store)
			if !assert.ErrorIs(t, err, storeErr) {
				return
			}
		})
	})
}
