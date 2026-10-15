// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package bedrock

import "fmt"

// ExitStatus is the status an App exits with.
//
// Its code is the exit status of the OS process, so it must lie in
// 0..255. The codes 0..125 are recommended, since a shell reports the
// higher ones for its own failures and for processes killed by a signal.
// The zero ExitStatus means success: its code is 0 and it has no cause.
//
// A non-zero ExitStatus can only be built with [Exit] or [ExitWithCause],
// both of which panic on a code outside 0..255.
type ExitStatus struct {
	code  int
	cause error
}

// Exit returns an ExitStatus with the given code and no cause.
//
// The code is the exit status of the OS process. It must lie in 0..255,
// and 0..125 is recommended. Exit panics on a code outside 0..255, naming
// the code, so an invalid code is caught at the return statement that
// produced it instead of being silently truncated to 8 bits by the OS.
func Exit(code int) ExitStatus {
	return ExitWithCause(code, nil)
}

// ExitWithCause returns an ExitStatus with the given code and the error
// that caused the App to exit.
//
// The code has the same range as for [Exit], and ExitWithCause panics on
// a code outside 0..255. The panic value is an error naming the code that
// wraps cause when it is non-nil.
//
// Any code may carry a cause, including 0: a graceful shutdown can return
// ExitWithCause(0, context.Cause(ctx)) so that a test can assert which
// signal stopped the App. The cause is for tests and other callers of
// [App.Run]; [Main] never reads it.
func ExitWithCause(code int, cause error) ExitStatus {
	if code < 0 || code > 255 {
		if cause == nil {
			panic(fmt.Errorf("bedrock: exit code %d is outside the range 0..255", code))
		}
		panic(fmt.Errorf("bedrock: exit code %d is outside the range 0..255: %w", code, cause))
	}
	return ExitStatus{code: code, cause: cause}
}

// Code returns the exit code, which is the exit status of the OS process.
// It is always in 0..255.
func (s ExitStatus) Code() int {
	return s.code
}

// Cause returns the error that caused the App to exit, or nil if there is
// none.
//
// The cause exists so that tests and other callers of [App.Run] can assert
// why an App exited, for example with errors.Is. [Main] ignores it and
// prints nothing, so an App reports anything a user should see on its own
// stderr.
func (s ExitStatus) Cause() error {
	return s.cause
}
