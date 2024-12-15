// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package config

import (
	"errors"
	"io"
	"io/fs"
	"testing"

	"github.com/stretchr/testify/assert"
)

type fsFunc func(string) (fs.File, error)

func (f fsFunc) Open(path string) (fs.File, error) {
	return f(path)
}

func TestFileReader_Read(t *testing.T) {
	t.Run("will return an error", func(t *testing.T) {
		t.Run("if the fs.FS fails to open the file", func(t *testing.T) {
			openErr := errors.New("failed to open")
			fs := fsFunc(func(s string) (fs.File, error) {
				return nil, openErr
			})

			r := NewFileReader(fs, "config.yaml")
			_, err := io.ReadAll(r)
			if !assert.ErrorIs(t, err, openErr) {
				return
			}
		})
	})
}

func TestFileReader_Close(t *testing.T) {
	t.Run("will not return an error", func(t *testing.T) {
		t.Run("if Close is called before the underlying file has been opened", func(t *testing.T) {
			fs := fsFunc(func(s string) (fs.File, error) {
				return nil, nil
			})

			r := NewFileReader(fs, "config.yaml")
			err := r.Close()
			if !assert.Nil(t, err) {
				return
			}
		})
	})
}
