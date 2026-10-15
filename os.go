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

// Process is what exec hands a process: its arguments, its environment,
// its standard streams and the delivery of its signals. It is the one
// value through which an [App] reaches the OS process it runs in.
//
// Every field is required, and the zero Process is invalid. This is
// documented rather than checked at runtime: a complete Process comes from
// [OSProcess] in production and from a test fake in tests, so code that
// receives a Process may use any field without a nil check.
type Process struct {
	// Args holds the command-line arguments, starting with the program
	// name, like os.Args. It is required.
	Args []string

	// Stdin is the standard input of the process. It is required.
	Stdin io.Reader

	// Stdout is the standard output of the process. It is required, and
	// it must be safe for concurrent writes if the App writes to it from
	// several goroutines, as an *os.File is.
	Stdout io.Writer

	// Stderr is the standard error of the process. It is required, and it
	// must be safe for concurrent writes if the App writes to it from
	// several goroutines, as an *os.File is.
	Stderr io.Writer

	// Environment provides the environment variables of the process. It
	// is required: OSEnvironment{} in production and a
	// [StaticEnvironment] in tests.
	Environment Environment

	// NotifyContext delivers signals to the process, with the semantics of
	// signal.NotifyContext, which is what [OSProcess] sets it to. It is
	// required.
	//
	// It returns a copy of parent that is done when one of signals
	// arrives, when stop is called, or when parent is done, whichever
	// happens first. Listing no signals means every signal. When a signal
	// cancels ctx, context.Cause(ctx) reports an error whose message is
	// "<signal> signal received", such as "interrupt signal received".
	//
	// os.Kill and SIGSTOP cannot be caught, so listing them has no effect.
	// stop must be called, typically with defer, to release the
	// registration. A signal that arrives after stop is called gets its
	// default action again, which for an interrupt is to exit.
	NotifyContext func(parent context.Context, signals ...os.Signal) (ctx context.Context, stop context.CancelFunc)
}

// OSEnvironment is an Environment backed by the environment of the current
// OS process. It is the Environment of [OSProcess].
type OSEnvironment struct{}

// Value implements the Environment interface.
func (OSEnvironment) Value(name string) (string, error) {
	v, ok := os.LookupEnv(name)
	if !ok {
		return "", ValueNotFoundError{Name: name}
	}
	return v, nil
}

// OSProcess returns the Process of the current OS process: os.Args,
// os.Stdin, os.Stdout, os.Stderr, OSEnvironment{} and
// signal.NotifyContext. Every field is set.
func OSProcess() Process {
	return Process{
		Args:          os.Args,
		Stdin:         os.Stdin,
		Stdout:        os.Stdout,
		Stderr:        os.Stderr,
		Environment:   OSEnvironment{},
		NotifyContext: signal.NotifyContext,
	}
}

// Main runs app on the current OS process and exits the process with the
// code of the [ExitStatus] it returns. It never returns.
//
// Main is a shim for the signature Go's own main function does not have:
//
//	func main() { bedrock.Main(app) }
//
// It is exactly os.Exit(app.Run(context.Background(), OSProcess()).Code()).
// Main ignores the status's [ExitStatus.Cause] and prints nothing, because
// the cause exists for tests; an App reports anything a user should see on
// [Process.Stderr]. A panic in app is not recovered, so it crashes the
// process with exit status 2.
func Main(app App) {
	os.Exit(app.Run(context.Background(), OSProcess()).Code())
}
