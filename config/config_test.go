// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package config

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"text/template"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

type readFunc func([]byte) (int, error)

func (f readFunc) Read(b []byte) (int, error) {
	return f(b)
}

func TestRead(t *testing.T) {
	t.Run("will return an error", func(t *testing.T) {
		t.Run("if it fails to read from the source reader", func(t *testing.T) {
			readErr := errors.New("read error")
			r := readFunc(func(b []byte) (int, error) {
				return 0, readErr
			})

			_, err := Read(r)
			if !assert.Equal(t, readErr, err) {
				return
			}
		})

		t.Run("if it's a malformed text template", func(t *testing.T) {
			r := strings.NewReader(`hello: {{world`)

			_, err := Read(r)
			if !assert.Error(t, err) {
				return
			}
		})

		t.Run("if it fails to execute the config as a text template", func(t *testing.T) {
			r := strings.NewReader(`hello: {{.World}}`)

			_, err := Read(r, Language(YAML))
			if !assert.IsType(t, template.ExecError{}, err) {
				return
			}
		})

		t.Run("if it's invalid yaml", func(t *testing.T) {
			r := strings.NewReader(`hello`)

			_, err := Read(r, Language(YAML))
			if !assert.IsType(t, viper.ConfigParseError{}, err) {
				return
			}
		})

		t.Run("if it's invalid json", func(t *testing.T) {
			r := strings.NewReader(`hello`)

			_, err := Read(r, Language(JSON))
			if !assert.IsType(t, viper.ConfigParseError{}, err) {
				return
			}
		})

		t.Run("if it's invalid toml", func(t *testing.T) {
			r := strings.NewReader(`hello`)

			_, err := Read(r, Language(TOML))
			if !assert.IsType(t, viper.ConfigParseError{}, err) {
				return
			}
		})
	})
}

func TestManager_Unmarshal(t *testing.T) {
	t.Run("will return an error", func(t *testing.T) {
		t.Run("if a nil result is provided", func(t *testing.T) {
			r := strings.NewReader(`hello: world`)
			m, err := Read(r, Language(YAML))
			if !assert.Nil(t, err) {
				return
			}

			var v any
			err = m.Unmarshal(v)
			if !assert.Error(t, err) {
				return
			}
		})
	})

	t.Run("will unmarshal time.Duration", func(t *testing.T) {
		t.Run("if the value is provided in string format", func(t *testing.T) {
			r := strings.NewReader(`duration: 10s`)
			m, err := Read(r, Language(YAML))
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
			r := strings.NewReader(fmt.Sprintf(`duration: %d`, 10*time.Second))
			m, err := Read(r, Language(YAML))
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
}
