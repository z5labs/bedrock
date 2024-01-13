// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package config

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"text/template"
	"time"

	"github.com/mitchellh/mapstructure"
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

func TestMerge(t *testing.T) {
	t.Run("will return an error", func(t *testing.T) {
		t.Run("if it fails to read the first config", func(t *testing.T) {
			var cfg Manager
			r := strings.NewReader(`hello`)
			_, err := Merge(cfg, r, Language(YAML))
			if !assert.IsType(t, viper.ConfigParseError{}, err) {
				return
			}
		})

		t.Run("if it viper fails to read from the io.Reader", func(t *testing.T) {
			base := strings.NewReader(`hello: world`)
			m, err := Read(base, Language(YAML))
			if !assert.Nil(t, err) {
				return
			}

			r := strings.NewReader(`hello`)
			_, err = Merge(m, r)
			if !assert.IsType(t, viper.ConfigParseError{}, err) {
				return
			}
		})
	})

	t.Run("will overwrite base value", func(t *testing.T) {
		t.Run("if the new reader contains the same key", func(t *testing.T) {
			base := strings.NewReader(`hello: world`)
			m, err := Read(base, Language(YAML))
			if !assert.Nil(t, err) {
				return
			}

			r := strings.NewReader(`hello: bye`)
			m, err = Merge(m, r)
			if !assert.Nil(t, err) {
				return
			}
			if !assert.Equal(t, "bye", m.GetString("hello")) {
				return
			}
		})
	})

	t.Run("will not overwrite base value", func(t *testing.T) {
		t.Run("if the new reader does not contain the same key", func(t *testing.T) {
			base := strings.NewReader(`hello: world`)
			m, err := Read(base, Language(YAML))
			if !assert.Nil(t, err) {
				return
			}

			r := strings.NewReader(`good: bye`)
			m, err = Merge(m, r)
			if !assert.Nil(t, err) {
				return
			}
			if !assert.Equal(t, "world", m.GetString("hello")) {
				return
			}
			if !assert.Equal(t, "bye", m.GetString("good")) {
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

		t.Run("if the encoding.TextUnmarshaler fails to UnmarshalText", func(t *testing.T) {
			r := strings.NewReader(`value: "10"`)
			m, err := Read(r, Language(YAML))
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

			var me *mapstructure.Error
			if !assert.ErrorAs(t, err, &me) {
				return
			}
			errs := me.WrappedErrors()
			if !assert.Len(t, errs, 1) {
				return
			}
			if !assert.Contains(t, errs[0].Error(), unmarshalErr.Error()) {
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

	t.Run("will unmarshal encoding.TextUnmarshaler", func(t *testing.T) {
		t.Run("if the value is a string", func(t *testing.T) {
			r := strings.NewReader(`value: "10"`)
			m, err := Read(r, Language(YAML))
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

func TestMapEnv(t *testing.T) {
	t.Run("will ignore malformed pairs", func(t *testing.T) {
		t.Run("if there is no '=' separating the key and value", func(t *testing.T) {
			pairs := []string{"hello=world", "good bye"}
			env := mapEnv(pairs)
			if !assert.Less(t, len(env), len(pairs)) {
				return
			}
			if !assert.Contains(t, env, "hello") {
				return
			}
		})
	})
}
