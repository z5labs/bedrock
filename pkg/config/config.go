// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package config

import (
	"bytes"
	"encoding"
	"errors"
	"io"
	"os"
	"reflect"
	"strings"
	"text/template"
	"time"

	"github.com/mitchellh/mapstructure"
	"github.com/spf13/viper"
)

// Manager stores config values and provides helpers for
// bridging the raw config values into Go types.
type Manager struct {
	*viper.Viper
}

// ReadOption configures different properties of the reader.
type ReadOption func(*reader)

// LanguageType configures the expected language the source is encoded in.
type LanguageType string

const (
	YAML LanguageType = "yaml"
	JSON LanguageType = "json"
	TOML LanguageType = "toml"
)

// Language sets which language the config source uses.
func Language(lang LanguageType) ReadOption {
	return func(r *reader) {
		r.lang = lang
	}
}

type reader struct {
	lang LanguageType
	env  map[string]string
}

// Read parses the data from r and stores the config values in the returned Manager.
func Read(r io.Reader, opts ...ReadOption) (Manager, error) {
	env := mapEnv(os.Environ())

	rd := reader{
		lang: YAML,
		env:  env,
	}
	for _, opt := range opts {
		opt(&rd)
	}

	v := viper.New()
	v.SetConfigType(string(rd.lang))
	err := rd.read(v, r)
	if err != nil {
		return Manager{}, err
	}
	return Manager{Viper: v}, nil
}

// Merge allows you to merge another config into an already existing one.
func Merge(m Manager, r io.Reader, opts ...ReadOption) (Manager, error) {
	if m.Viper == nil {
		return Read(r, opts...)
	}

	env := mapEnv(os.Environ())
	rd := reader{
		lang: YAML,
		env:  env,
	}
	for _, opt := range opts {
		opt(&rd)
	}

	var buf bytes.Buffer
	err := rd.renderTemplate(&buf, r)
	if err != nil {
		return m, err
	}

	return m, m.MergeConfig(&buf)
}

// Unmarshal unmarshals the config into the value pointed to by v.
func (m Manager) Unmarshal(v interface{}) error {
	dec, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		TagName: "config",
		Result:  v,
		DecodeHook: composeDecodeHooks(
			textUnmarshalerHookFunc(),
			timeDurationHookFunc(),
		),
	})
	if err != nil {
		return err
	}
	return dec.Decode(m.AllSettings())
}

var errInvalidDecodeCondition = errors.New("invalid decode condition")

type multiError struct {
	errors []error
}

func (e multiError) Error() string {
	ss := make([]string, len(e.errors))
	for i, e := range e.errors {
		ss[i] = e.Error()
	}
	return strings.Join(ss, "\n")
}

func composeDecodeHooks(hs ...mapstructure.DecodeHookFunc) mapstructure.DecodeHookFuncValue {
	return func(f, t reflect.Value) (any, error) {
		var errs []error
		for _, h := range hs {
			v, err := mapstructure.DecodeHookExec(h, f, t)
			if err == nil {
				return v, nil
			}
			if err == errInvalidDecodeCondition {
				continue
			}
			errs = append(errs, err)
		}
		if len(errs) == 0 {
			return f.Interface(), nil
		}
		return nil, multiError{errors: errs}
	}
}

func textUnmarshalerHookFunc() mapstructure.DecodeHookFuncType {
	return func(f reflect.Type, t reflect.Type, data any) (any, error) {
		if f.Kind() != reflect.String {
			return nil, errInvalidDecodeCondition
		}
		result := reflect.New(t).Interface()
		u, ok := result.(encoding.TextUnmarshaler)
		if !ok {
			return nil, errInvalidDecodeCondition
		}
		err := u.UnmarshalText([]byte(data.(string)))
		if err != nil {
			return nil, err
		}
		return result, nil
	}
}

func timeDurationHookFunc() mapstructure.DecodeHookFuncType {
	return func(f reflect.Type, t reflect.Type, data any) (any, error) {
		if t != reflect.TypeOf(time.Duration(0)) {
			return nil, errInvalidDecodeCondition
		}

		switch f.Kind() {
		case reflect.String:
			return time.ParseDuration(data.(string))
		case reflect.Int:
			return time.Duration(int64(data.(int))), nil
		default:
			return nil, errInvalidDecodeCondition
		}
	}
}

func (rd reader) renderTemplate(dst io.Writer, src io.Reader) error {
	var sb strings.Builder
	_, err := io.Copy(&sb, src)
	if err != nil {
		return err
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
		return err
	}

	return tmpl.Execute(dst, struct{}{})
}

func (rd reader) read(v *viper.Viper, r io.Reader) error {
	var buf bytes.Buffer
	err := rd.renderTemplate(&buf, r)
	if err != nil {
		return err
	}

	v.SetConfigType(string(rd.lang))
	err = v.ReadConfig(&buf)
	if err != nil {
		return err
	}
	return nil
}

// envVars = pairs formatted with a '=' between the key and value
func mapEnv(envVars []string) map[string]string {
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
