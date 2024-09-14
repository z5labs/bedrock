// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package internal

import (
	"io"
	"os"

	"github.com/z5labs/bedrock/pkg/config"
)

func ConfigSource(r io.Reader) config.Source {
	return config.FromYaml(
		config.RenderTextTemplate(
			r,
			config.TemplateFunc("env", os.Getenv),
			config.TemplateFunc("default", func(s string, v any) any {
				if len(s) == 0 {
					return v
				}
				return s
			}),
		),
	)
}
