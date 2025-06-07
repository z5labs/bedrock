// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package config

import (
	"fmt"
	"io"

	"github.com/z5labs/sdk-go/try"
	"gopkg.in/yaml.v3"
)

// Yaml represents a Source where its underlying format is YAML.
type Yaml struct {
	r io.Reader
}

// FromYaml returns a source which will apply its config
// from YAML values parsed from the given io.Reader.
func FromYaml(r io.Reader) Yaml {
	return Yaml{r: r}
}

// InvalidYamlError occurs if the underlying io.Reader contains invalid YAML.
type InvalidYamlError struct {
	cause error
}

// Error implements the error interface.
func (e InvalidYamlError) Error() string {
	return fmt.Sprintf("invalid yaml: %s", e.cause)
}

// Unwrap implmeents the implicit interface used by errors.Is and errors.As.
func (e InvalidYamlError) Unwrap() error {
	return e.cause
}

// Apply implements the Source interface.
func (src Yaml) Apply(store Store) (err error) {
	c, _ := src.r.(io.Closer)
	defer try.Close(&err, c)

	b, err := io.ReadAll(src.r)
	if err != nil {
		return err
	}

	m := make(map[string]any)
	err = yaml.Unmarshal(b, &m)
	if err != nil {
		return InvalidYamlError{cause: err}
	}
	return Map(m).Apply(store)
}
