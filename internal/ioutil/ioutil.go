// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package ioutil

import (
	"errors"
	"fmt"
	"io"
)

type CloseError struct {
	Cause error
}

func (e CloseError) Error() string {
	return fmt.Sprintf("failed to close reader: %s", e.Cause)
}

func (e CloseError) Unwrap() error {
	return e.Cause
}

// ReadAllAndTryClose
func ReadAllAndTryClose(r io.Reader) (_ []byte, err error) {
	defer tryClose(&err, r)
	return io.ReadAll(r)
}

// CopyAndTryClose
func CopyAndTryClose(dst io.Writer, src io.Reader) (_ int64, err error) {
	defer tryClose(&err, src)
	return io.Copy(dst, src)
}

func tryClose(err *error, r io.Reader) {
	rc, ok := r.(io.ReadCloser)
	if !ok {
		return
	}

	closeErr := rc.Close()
	if closeErr == nil {
		return
	}

	cerr := CloseError{
		Cause: closeErr,
	}
	if err == nil {
		*err = cerr
	}
	*err = errors.Join(*err, cerr)
}
