// Copyright (c) 2025 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package try

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRecover(t *testing.T) {
	t.Run("will update the error ref value", func(t *testing.T) {
		t.Run("if a panic is successfully recovered from and the ref is set to nil", func(t *testing.T) {
			f := func() (err error) {
				defer Recover(&err)
				panic("hello world")
			}

			err := f()

			var perr PanicError
			if !assert.ErrorAs(t, err, &perr) {
				return
			}
			if !assert.NotEmpty(t, perr.Error()) {
				return
			}
			if !assert.Equal(t, "hello world", perr.Value) {
				return
			}
		})

		t.Run("if a panic is successfully recovered from and the ref is set to a non-nil value", func(t *testing.T) {
			funcErr := errors.New("error value")
			panicErr := errors.New("panic error")
			f := func() (err error) {
				defer Recover(&err)
				err = funcErr
				panic(panicErr)
			}

			err := f()

			if !assert.ErrorIs(t, err, funcErr) {
				return
			}

			var perr PanicError
			if !assert.ErrorAs(t, err, &perr) {
				return
			}
			if !assert.NotEmpty(t, perr.Error()) {
				return
			}
			if !assert.ErrorIs(t, perr, panicErr) {
				return
			}
		})
	})

	t.Run("will not update the error ref value", func(t *testing.T) {
		t.Run("if no panic is occurred", func(t *testing.T) {
			f := func() (err error) {
				defer Recover(&err)
				return nil
			}

			err := f()
			if !assert.Nil(t, err) {
				return
			}
		})
	})
}

type closeFunc func() error

func (f closeFunc) Close() error {
	return f()
}

func TestClose(t *testing.T) {
	t.Run("will update the error ref value", func(t *testing.T) {
		t.Run("if the close fails and the ref value is nil", func(t *testing.T) {
			closeErr := errors.New("close failed")
			c := closeFunc(func() error {
				return closeErr
			})

			f := func() (err error) {
				defer Close(&err, c)
				return nil
			}

			err := f()

			var cerr CloseError
			if !assert.ErrorAs(t, err, &cerr) {
				return
			}
			if !assert.NotEmpty(t, cerr.Error()) {
				return
			}
			if !assert.ErrorIs(t, cerr, closeErr) {
				return
			}
		})

		t.Run("if the close fails and the ref value is non-nil", func(t *testing.T) {
			closeErr := errors.New("close failed")
			c := closeFunc(func() error {
				return closeErr
			})

			funcErr := errors.New("func error")
			f := func() (err error) {
				defer Close(&err, c)
				return funcErr
			}

			err := f()

			if !assert.ErrorIs(t, err, funcErr) {
				return
			}

			var cerr CloseError
			if !assert.ErrorAs(t, err, &cerr) {
				return
			}
			if !assert.NotEmpty(t, cerr.Error()) {
				return
			}
			if !assert.ErrorIs(t, cerr, closeErr) {
				return
			}
		})
	})

	t.Run("will change the error ref value", func(t *testing.T) {
		t.Run("if the value is not an io.Closer", func(t *testing.T) {
			funcErr := errors.New("func error")
			f := func() (err error) {
				defer Close(&err, nil)
				return funcErr
			}

			err := f()
			if !assert.ErrorIs(t, err, funcErr) {
				return
			}
		})

		t.Run("if Close succeeds", func(t *testing.T) {
			c := closeFunc(func() error {
				return nil
			})

			funcErr := errors.New("func error")
			f := func() (err error) {
				defer Close(&err, c)
				return funcErr
			}

			err := f()
			if !assert.ErrorIs(t, err, funcErr) {
				return
			}
		})
	})
}
