// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package config

import (
	"io"
	"io/fs"
	"sync"
)

// FileReader is an io.Reader that handles opening a file for reading automatically.
type FileReader struct {
	path string

	openOnce sync.Once
	fs       fs.FS
	file     io.ReadCloser
}

// NewFileReader configures a FileReader.
func NewFileReader(fs fs.FS, path string) *FileReader {
	return &FileReader{
		path: path,
		fs:   fs,
	}
}

// Read implements the Read interface.
func (r *FileReader) Read(b []byte) (int, error) {
	var err error
	r.openOnce.Do(func() {
		r.file, err = r.fs.Open(r.path)
	})
	if err != nil {
		return 0, err
	}
	return r.file.Read(b)
}

// Close implements the io.Closer interface.
func (r *FileReader) Close() error {
	if r.file == nil {
		return nil
	}

	err := r.file.Close()
	r.file = nil
	return err
}
