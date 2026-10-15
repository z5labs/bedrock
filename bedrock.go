// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package bedrock models an application as an OS process.
package bedrock

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
)

// App is an application that runs as an OS process.
type App interface {
	Run(Context) ExitStatus
}

// AppFunc is a function that implements App.
type AppFunc func(Context) ExitStatus

// Run implements the App interface.
func (f AppFunc) Run(ctx Context) ExitStatus {
	return f(ctx)
}

// ExitStatus is the status an App exits with.
type ExitStatus struct {
	code  int
	cause error
}

// Exit returns an ExitStatus with the given code.
func Exit(code int) ExitStatus {
	return ExitStatus{code: code}
}

// ExitWithCause returns an ExitStatus with the given code and the error
// that caused the App to exit.
func ExitWithCause(code int, cause error) ExitStatus {
	return ExitStatus{code: code, cause: cause}
}

// Code returns the exit code.
func (s ExitStatus) Code() int {
	return s.code
}

// Cause returns the error that caused the App to exit, if any.
func (s ExitStatus) Cause() error {
	return s.cause
}

// Main runs the App with the Context of the current OS process and exits with its code.
func Main(app App) {
	os.Exit(app.Run(OsContext()).Code())
}

// Context is everything an App is given by the OS process it runs in.
type Context struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	Environment Environment

	NotifyOnInterrupt func(parent context.Context, signals ...os.Signal) (context.Context, context.CancelFunc)
}

// Environment provides the environment variables of a process.
type Environment interface {
	Value(name string) (string, error)
}

// ValueNotFoundError is returned by an Environment when a variable is not set.
type ValueNotFoundError struct {
	Name string
}

// Error implements the error interface.
func (e ValueNotFoundError) Error() string {
	return fmt.Sprintf("environment variable not found: %s", e.Name)
}

// StaticEnvironment is an Environment backed by a fixed set of values.
type StaticEnvironment struct {
	values map[string]string
}

// NewStaticEnvironment returns a StaticEnvironment backed by the given values.
func NewStaticEnvironment(values map[string]string) StaticEnvironment {
	return StaticEnvironment{values: values}
}

// Value implements the Environment interface.
func (e StaticEnvironment) Value(name string) (string, error) {
	v, ok := e.values[name]
	if !ok {
		return "", ValueNotFoundError{Name: name}
	}
	return v, nil
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
