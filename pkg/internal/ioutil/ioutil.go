// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package ioutil

import (
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

func ReadAllAndClose(r io.Reader) ([]byte, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return b, err
	}

	rc, ok := r.(io.ReadCloser)
	if !ok {
		return b, nil
	}

	err = rc.Close()
	if err != nil {
		return nil, CloseError{Cause: err}
	}
	return b, nil
}

func CopyAndClose(w io.Writer, r io.Reader) (int64, error) {
	n, err := io.Copy(w, r)
	if err != nil {
		return n, err
	}

	rc, ok := r.(io.ReadCloser)
	if !ok {
		return n, nil
	}

	err = rc.Close()
	if err != nil {
		return n, CloseError{Cause: err}
	}
	return n, nil
}
