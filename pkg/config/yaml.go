// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package config

import "io"

// YamlSource
type YamlSource struct {
	r io.Reader
}

func NewYamlSource(r io.Reader) YamlSource {
	return YamlSource{r: r}
}

func (src YamlSource) Apply(store Store) error {
	return nil
}
