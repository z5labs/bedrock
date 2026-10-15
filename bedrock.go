// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package bedrock models an application as an OS process.
package bedrock

import (
	"fmt"
)

// App is an application that runs as an OS process.
type App interface {
	// Run runs the App with the Context of the process it runs in and
	// returns the status the process exits with.
	//
	// The code of the returned ExitStatus is the exit status of the
	// process, with 0..125 recommended, and the zero ExitStatus means
	// success. Its cause is for tests and other callers of Run: [Main]
	// ignores it, so an App reports anything a user should see on its own
	// stderr.
	Run(Context) ExitStatus
}

// AppFunc is a function that implements App.
type AppFunc func(Context) ExitStatus

// Run implements the App interface.
func (f AppFunc) Run(ctx Context) ExitStatus {
	return f(ctx)
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
