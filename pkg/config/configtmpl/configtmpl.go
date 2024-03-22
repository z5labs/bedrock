// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package configtmpl provides template functions for ue in config source templates.
package configtmpl

import (
	"os"
	"reflect"
	"strings"
	"sync"
)

var envReadOnce sync.Once
var env map[string]string

// Env returns the environment variable value for the given key
// or an empty string, if the environment variable does not exist.
func Env(key string) string {
	envReadOnce.Do(func() {
		env = mapEnv(os.Environ())
	})
	return env[key]
}

func mapEnv(keyValues []string) map[string]string {
	m := make(map[string]string, len(keyValues))
	for _, s := range keyValues {
		key, value, ok := strings.Cut(s, "=")
		if !ok {
			continue
		}
		m[key] = value
	}
	return m
}

// Default returns the provided def value if v is either nil or the zero value for its type.
func Default(def, v any) any {
	if v == nil {
		return def
	}
	val := reflect.ValueOf(v)
	if val.IsZero() {
		return def
	}
	return v
}
