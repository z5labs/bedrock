// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package config

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/z5labs/sdk-go/try"
)

// Json represents a Source where its underlying format is JSON.
type Json struct {
	r io.Reader
}

// FromJson returns a source which will apply its config
// from JSON values parsed from the given io.Reader.
func FromJson(r io.Reader) Json {
	return Json{r: r}
}

// InvalidJsonError occurs if the underlying io.Reader contains invalid JSON.
type InvalidJsonError struct {
	cause error
}

// Error implements the error interface.
func (e InvalidJsonError) Error() string {
	return fmt.Sprintf("invalid json: %s", e.cause)
}

// Unwrap implmeents the implicit interface used by errors.Is and errors.As.
func (e InvalidJsonError) Unwrap() error {
	return e.cause
}

// Apply implements the Source interface.
func (src Json) Apply(store Store) (err error) {
	c, _ := src.r.(io.Closer)
	defer try.Close(&err, c)

	b, err := io.ReadAll(src.r)
	if err != nil {
		return err
	}

	m := make(map[string]any)
	err = json.Unmarshal(b, &m)
	if err != nil {
		return InvalidJsonError{cause: err}
	}
	return Map(m).Apply(store)
}
