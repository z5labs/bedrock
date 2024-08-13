// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package config

import (
	"io"
	"text/template"
)

type RenderTextTemplateOption func(*TextTemplateRenderer)

func TemplateFunc(name string, f any) RenderTextTemplateOption {
	return func(ttr *TextTemplateRenderer) {
		ttr.funcs[name] = f
	}
}

type TextTemplateRenderer struct {
	r     io.Reader
	funcs template.FuncMap
}

func RenderTextTemplate(r io.Reader, opts ...RenderTextTemplateOption) *TextTemplateRenderer {
	ttr := &TextTemplateRenderer{
		r:     r,
		funcs: make(template.FuncMap),
	}
	for _, opt := range opts {
		opt(ttr)
	}
	return ttr
}

// Read implements the read interface.
func (ttr *TextTemplateRenderer) Read(b []byte) (int, error) {
	return 0, nil
}
