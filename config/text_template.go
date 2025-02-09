// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"text/template"

	"github.com/z5labs/bedrock/internal/try"
)

// RenderTextTemplateOption represents options for configuring the TextTemplateRenderer.
type RenderTextTemplateOption func(*TextTemplateRenderer)

// TemplateFunc registers the given function, f, for use in the config
// template via the given name.
func TemplateFunc(name string, f any) RenderTextTemplateOption {
	return func(ttr *TextTemplateRenderer) {
		ttr.funcs[name] = f
	}
}

// TemplateDelims sets the action delimiters to the specified strings.
// Nested template definitions will inherit the settings. An empty delimiter
// stands for the corresponding default: {{ or }}.
func TemplateDelims(left, right string) RenderTextTemplateOption {
	return func(ttr *TextTemplateRenderer) {
		ttr.leftDelim = left
		ttr.rightDelim = right
	}
}

// TextTemplateRenderer is an io.Reader that renders a text/template from
// a given io.Reader. The rendered template can then be read via [TextTemplateRenderer.Read].
type TextTemplateRenderer struct {
	r io.Reader

	leftDelim  string
	rightDelim string
	funcs      template.FuncMap
	renderOnce sync.Once
	buf        bytes.Buffer
}

// RenderTextTemplate configures a TextTemplateRenderer.
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

// TextTemplateParseError occurs when the config template fails to be parsed.
type TextTemplateParseError struct {
	Cause error
}

// Error implements the error interface.
func (e TextTemplateParseError) Error() string {
	return fmt.Sprintf("failed to parse config template: %s", e.Cause)
}

// Unwrap implements the implicit interface used by errors.Is and errors.As.
func (e TextTemplateParseError) Unwrap() error {
	return e.Cause
}

// TextTemplateExecError occurs when a template fails to execute. Most
// likely cause is using template functions returning an error or panicing.
type TextTemplateExecError struct {
	Cause error
}

// Error implements the error interface.
func (e TextTemplateExecError) Error() string {
	return fmt.Sprintf("failed to exec config template: %s", e.Cause)
}

// Unwrap implements the implicit interface used by errors.Is and errors.As.
func (e TextTemplateExecError) Unwrap() error {
	return e.Cause
}

// Read implements the read interface.
func (ttr *TextTemplateRenderer) Read(b []byte) (int, error) {
	var err error
	ttr.renderOnce.Do(func() {
		defer try.Close(&err, ttr.r)

		var sb strings.Builder
		_, err = io.Copy(&sb, ttr.r)
		if err != nil && !errors.Is(err, try.CloseError{}) {
			// We can ignore ioutil.CloseError because we've successfully
			// read the file contents and closing is just a nice clean up
			// practice to follow but not mandatory.
			return
		}

		var tmpl *template.Template
		tmpl, err = template.New("config").
			Delims(ttr.leftDelim, ttr.rightDelim).
			Funcs(ttr.funcs).
			Parse(sb.String())
		if err != nil {
			err = TextTemplateParseError{Cause: err}
			return
		}

		err = tmpl.Execute(&ttr.buf, struct{}{})
		if err != nil {
			err = TextTemplateExecError{Cause: err}
			return
		}
	})
	if err != nil {
		return 0, err
	}
	return ttr.buf.Read(b)
}
