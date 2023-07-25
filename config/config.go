// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package config

import (
	"bytes"
	"io"
	"os"
	"strings"
	"text/template"

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
	ctx  Context
}

// Read
func Read(r io.Reader, opts ...ReadOption) (Manager, error) {
	env := readEnv()

	rd := reader{
		lang: YAML,
		ctx: Context{
			Env: env,
		},
	}
	for _, opt := range opts {
		opt(&rd)
	}

	return rd.read(r)
}

type Context struct {
	Env map[string]string
}

func (rd reader) read(r io.Reader) (Manager, error) {
	var sb strings.Builder
	_, err := io.Copy(&sb, r)
	if err != nil {
		return Manager{}, err
	}
	s := sb.String()

	tmpl, err := template.New("config").Parse(s)
	if err != nil {
		return Manager{}, err
	}

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, rd.ctx)
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
