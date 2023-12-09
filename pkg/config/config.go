// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package config

import (
	"bytes"
	"io"
	"os"
	"reflect"
	"strings"
	"text/template"
	"time"

	"github.com/mitchellh/mapstructure"
	"github.com/spf13/viper"
)

// Manager
type Manager struct {
	*viper.Viper
}

type ReadOption func(*reader)

type LanguageType string

const (
	YAML LanguageType = "yaml"
	JSON LanguageType = "json"
	TOML LanguageType = "toml"
)

func Language(lang LanguageType) ReadOption {
	return func(r *reader) {
		r.lang = lang
	}
}

type reader struct {
	lang LanguageType
	env  map[string]string
}

// Read
func Read(r io.Reader, opts ...ReadOption) (Manager, error) {
	env := readEnv()

	rd := reader{
		lang: YAML,
		env:  env,
	}
	for _, opt := range opts {
		opt(&rd)
	}

	return rd.read(r)
}

// Unmarshal
func (m Manager) Unmarshal(v interface{}) error {
	dec, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		TagName: "config",
		Result:  v,
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			decodeTimeDuration(),
		),
	})
	if err != nil {
		return err
	}
	return dec.Decode(m.AllSettings())
}

func decodeTimeDuration() mapstructure.DecodeHookFunc {
	return func(f reflect.Type, t reflect.Type, data any) (any, error) {
		if t != reflect.TypeOf(time.Duration(0)) {
			return data, nil
		}

		switch f.Kind() {
		case reflect.String:
			return time.ParseDuration(data.(string))
		case reflect.Int64:
			return time.Duration(data.(int64)), nil
		default:
			return data, nil
		}
	}
}

func (rd reader) read(r io.Reader) (Manager, error) {
	var sb strings.Builder
	_, err := io.Copy(&sb, r)
	if err != nil {
		return Manager{}, err
	}
	s := sb.String()

	funcs := template.FuncMap{
		"env": func(key string) string {
			return rd.env[key]
		},
		"default": func(def any, v string) any {
			if len(v) == 0 {
				return def
			}
			return v
		},
	}

	tmpl, err := template.New("config").Funcs(funcs).Parse(s)
	if err != nil {
		return Manager{}, err
	}

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, struct{}{})
	if err != nil {
		return Manager{}, err
	}

	v := viper.New()
	v.SetConfigType(string(rd.lang))
	err = v.ReadConfig(&buf)
	if err != nil {
		return Manager{}, err
	}
	return Manager{Viper: v}, nil
}

func readEnv() map[string]string {
	envVars := os.Environ()
	envs := make(map[string]string, len(envVars))
	for _, envVar := range envVars {
		key, value, ok := strings.Cut(envVar, "=")
		if !ok {
			continue
		}
		envs[key] = value
	}
	return envs
}
