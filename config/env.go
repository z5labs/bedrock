// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package config

import (
	"os"
	"strings"
)

// Env represents a Source where its underlying values
// are extracted from environment variables.
type Env struct {
	environ func() []string
}

// FromEnv returns a Source which will apply its config
// from the environment variables available to the
// current process.
func FromEnv() Env {
	return Env{
		environ: os.Environ,
	}
}

// Apply implements the Source interface.
func (src Env) Apply(store Store) error {
	m := make(Map)
	env := src.environ()
	for _, pair := range env {
		k, v, ok := strings.Cut(pair, "=")
		if !ok {
			continue
		}
		m[k] = v
	}
	return m.Apply(store)
}
