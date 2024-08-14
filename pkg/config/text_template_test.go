// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package config

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTextTemplateRenderer_Read(t *testing.T) {
	t.Run("will return an error", func(t *testing.T) {
		t.Run("if the underlying io.Reader fails", func(t *testing.T) {
			readErr := errors.New("failed to read")
			r := readFunc(func(b []byte) (int, error) {
				return 0, readErr
			})

			ttr := RenderTextTemplate(r)
			_, err := io.ReadAll(ttr)
			if !assert.ErrorIs(t, err, readErr) {
				return
			}
		})

		t.Run("if the underlying io.Reader contains an invalid text/template", func(t *testing.T) {
			r := strings.NewReader(`{{ hello`)

			ttr := RenderTextTemplate(r)
			_, err := io.ReadAll(ttr)

			var ierr TextTemplateParseError
			if !assert.ErrorAs(t, err, &ierr) {
				return
			}
			if !assert.NotEmpty(t, ierr.Error()) {
				return
			}
			if !assert.Error(t, ierr.Unwrap()) {
				return
			}
		})

		t.Run("if the parsed text/template fails to execute", func(t *testing.T) {
			r := strings.NewReader(`{{ hello }}`)

			ttr := RenderTextTemplate(
				r,
				TemplateFunc("hello", func() string {
					panic("ahhhh")
				}),
			)
			_, err := io.ReadAll(ttr)

			var ierr TextTemplateExecError
			if !assert.ErrorAs(t, err, &ierr) {
				return
			}
			if !assert.NotEmpty(t, ierr.Error()) {
				return
			}
			if !assert.Error(t, ierr.Unwrap()) {
				return
			}
		})
	})
}
