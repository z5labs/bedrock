// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package bedrock

import (
	"context"
	"io"
	"os"
	"os/signal"
)

// This file is the OS edge: it is the only non-test file in the package
// that imports os or os/signal.

// Context is everything an App is given by the OS process it runs in.
type Context struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	Environment Environment

	NotifyOnInterrupt func(parent context.Context, signals ...os.Signal) (context.Context, context.CancelFunc)
}

// OsEnvironment is an Environment backed by the environment of the current process.
type OsEnvironment struct{}

// Value implements the Environment interface.
func (OsEnvironment) Value(name string) (string, error) {
	v, ok := os.LookupEnv(name)
	if !ok {
		return "", ValueNotFoundError{Name: name}
	}
	return v, nil
}

// OsContext returns the Context of the current OS process.
func OsContext() Context {
	return Context{
		Stdin:             os.Stdin,
		Stdout:            os.Stdout,
		Stderr:            os.Stderr,
		Environment:       OsEnvironment{},
		NotifyOnInterrupt: signal.NotifyContext,
	}
}

// Main runs app on the current OS process and exits the process with the
// code of the [ExitStatus] it returns. It never returns.
//
// Main is a shim for the signature Go's own main function does not have:
//
//	func main() { bedrock.Main(app) }
//
// It is exactly os.Exit(app.Run(OsContext()).Code()). Main ignores the
// status's [ExitStatus.Cause] and prints nothing, because the cause exists
// for tests; an App reports anything a user should see on its own stderr.
// A panic in app is not recovered, so it crashes the process with exit
// status 2.
func Main(app App) {
	os.Exit(app.Run(OsContext()).Code())
}
