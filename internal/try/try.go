// Copyright (c) 2025 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package try

import (
	"errors"
	"fmt"
	"io"
)

type PanicError struct {
	Value any
}

func (e PanicError) Error() string {
	return fmt.Sprintf("recovered from panic: %v", e.Value)
}

func (e PanicError) Unwrap() error {
	return e.Value.(error)
}

func Recover(err *error) {
	r := recover()
	if r == nil {
		return
	}

	perr := PanicError{
		Value: r,
	}
	if *err == nil {
		*err = perr
		return
	}
	*err = errors.Join(*err, perr)
}

type CloseError struct {
	Cause error
}

func (e CloseError) Error() string {
	return fmt.Sprintf("failed to close: %s", e.Cause)
}

func (e CloseError) Unwrap() error {
	return e.Cause
}

func Close(err *error, v any) {
	c, ok := v.(io.Closer)
	if !ok {
		return
	}

	cerr := c.Close()
	if cerr == nil {
		return
	}

	if *err == nil {
		*err = cerr
		return
	}
	*err = errors.Join(*err, cerr)
}
