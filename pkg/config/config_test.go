// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package config

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type sourceFunc func(Store) error

func (f sourceFunc) Apply(store Store) error {
	return f(store)
}

func TestRead(t *testing.T) {
	t.Run("will return an error", func(t *testing.T) {
		t.Run("if one of the Sources fails to apply itself to the store", func(t *testing.T) {
			srcErr := errors.New("failed to apply config")
			src := sourceFunc(func(s Store) error {
				return srcErr
			})

			_, err := Read(src)
			if !assert.ErrorIs(t, err, srcErr) {
				return
			}
		})
	})

	t.Run("will return empty Manager", func(t *testing.T) {
		t.Run("if no sources are provided", func(t *testing.T) {
			m, err := Read()
			if !assert.Nil(t, err) {
				return
			}
			if !assert.NotNil(t, m.store) {
				return
			}
			if !assert.Len(t, m.store, 0) {
				return
			}
		})
	})

	t.Run("will override config values", func(t *testing.T) {
		t.Run("if multiple sources are provided", func(t *testing.T) {
			m, err := Read(
				FromYaml(strings.NewReader("hello: alice")),
				FromYaml(strings.NewReader("hello: bob")),
			)
			if !assert.Nil(t, err) {
				return
			}

			var cfg struct {
				Hello string `config:"hello"`
			}
			err = m.Unmarshal(&cfg)
			if !assert.Nil(t, err) {
				return
			}
			if !assert.Equal(t, "bob", cfg.Hello) {
				return
			}
		})
	})

	t.Run("will be idempotent", func(t *testing.T) {
		t.Run("if a single Manager is used as the source", func(t *testing.T) {
			m, err := Read(FromYaml(strings.NewReader("hello: world")))
			if !assert.Nil(t, err) {
				return
			}

			m2, err := Read(m)
			if !assert.Nil(t, err) {
				return
			}
			if !assert.Equal(t, m, m2) {
				return
			}
		})
	})
}

type Custom struct {
	N int
}

func (c *Custom) UnmarshalText(b []byte) error {
	n, err := strconv.Atoi(string(b))
	if err != nil {
		return err
	}
	c.N = n
	return nil
}

var unmarshalErr = errors.New("failed to unmarshal")

type UnmarshalTextFailure struct{}

func (x UnmarshalTextFailure) UnmarshalText(b []byte) error {
	return unmarshalErr
}

func TestManager_Unmarshal(t *testing.T) {
	t.Run("will return an error", func(t *testing.T) {
		t.Run("if a nil result is provided", func(t *testing.T) {
			src := Map{
				"hello": "world",
			}

			m, err := Read(src)
			if !assert.Nil(t, err) {
				return
			}

			var v any
			err = m.Unmarshal(v)
			if !assert.Error(t, err) {
				return
			}
		})

		t.Run("if the encoding.TextUnmarshaler fails to UnmarshalText", func(t *testing.T) {
			src := Map{
				"value": "10",
			}

			m, err := Read(src)
			if !assert.Nil(t, err) {
				return
			}

			var cfg struct {
				Value UnmarshalTextFailure `config:"value"`
			}
			err = m.Unmarshal(&cfg)
			if !assert.Error(t, err) {
				return
			}
			if !assert.ErrorIs(t, err, unmarshalErr) {
				return
			}
		})
	})

	t.Run("will unmarshal time.Duration", func(t *testing.T) {
		t.Run("if the value is provided in string format", func(t *testing.T) {
			src := Map{
				"duration": "10s",
			}

			m, err := Read(src)
			if !assert.Nil(t, err) {
				return
			}

			var cfg struct {
				Duration time.Duration `config:"duration"`
			}
			err = m.Unmarshal(&cfg)
			if !assert.Nil(t, err) {
				return
			}
			if !assert.Equal(t, 10*time.Second, cfg.Duration) {
				return
			}
		})

		t.Run("if the value is provided in int format", func(t *testing.T) {
			src := Map{
				"duration": int(10 * time.Second),
			}

			m, err := Read(src)
			if !assert.Nil(t, err) {
				return
			}

			var cfg struct {
				Duration time.Duration `config:"duration"`
			}
			err = m.Unmarshal(&cfg)
			if !assert.Nil(t, err) {
				return
			}
			if !assert.Equal(t, 10*time.Second, cfg.Duration) {
				return
			}
		})
	})

	t.Run("will unmarshal encoding.TextUnmarshaler", func(t *testing.T) {
		t.Run("if the value is a string", func(t *testing.T) {
			src := Map{
				"value": "10",
			}

			m, err := Read(src)
			if !assert.Nil(t, err) {
				return
			}

			var cfg struct {
				Value Custom `config:"value"`
			}
			err = m.Unmarshal(&cfg)
			if !assert.Nil(t, err) {
				return
			}
			if !assert.Equal(t, 10, cfg.Value.N) {
				return
			}
		})
	})
}
