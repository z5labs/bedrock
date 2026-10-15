// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package bedrock

import (
	"context"
	"fmt"
	"maps"
)

// App is an application that runs as an OS process.
type App interface {
	// Run runs the App in the given [Process] and returns the status the
	// process exits with.
	//
	// ctx is the root context of the run: [Main] passes
	// context.Background() and a test passes t.Context(). An App that
	// listens for signals derives its context from ctx through
	// [Process.NotifyContext], so cancelling ctx reaches it too.
	//
	// The code of the returned ExitStatus is the exit status of the
	// process, with 0..125 recommended, and the zero ExitStatus means
	// success. Its cause is for tests and other callers of Run: [Main]
	// ignores it, so an App reports anything a user should see on
	// [Process.Stderr].
	Run(ctx context.Context, p Process) ExitStatus
}

// AppFunc is a function that implements App.
type AppFunc func(ctx context.Context, p Process) ExitStatus

// Run implements the App interface by calling f(ctx, p).
func (f AppFunc) Run(ctx context.Context, p Process) ExitStatus {
	return f(ctx, p)
}

// Environment provides the environment variables of a process.
//
// It is the only environment type in bedrock: [OSEnvironment] reads the
// environment of the current OS process and [StaticEnvironment] holds a
// fixed set of values for tests. An Environment offers lookup only, so it
// cannot be listed; see the package documentation for what that means for
// child processes.
type Environment interface {
	// Value returns the value of the named variable, or a
	// [ValueNotFoundError] naming it when it is not set.
	Value(name string) (string, error)
}

// ValueNotFoundError is returned by an Environment when a variable is not set.
type ValueNotFoundError struct {
	// Name is the name of the variable that is not set.
	Name string
}

// Error implements the error interface.
func (e ValueNotFoundError) Error() string {
	return fmt.Sprintf("environment variable not found: %s", e.Name)
}

// StaticEnvironment is an Environment backed by a fixed set of values. It
// is the Environment of a hand-built [Process] in tests.
//
// A StaticEnvironment is immutable, so it is safe for concurrent use. Build
// one with [NewStaticEnvironment]; the zero StaticEnvironment has no
// variables set.
type StaticEnvironment struct {
	values map[string]string
}

// NewStaticEnvironment returns a StaticEnvironment holding a copy of
// values. Changing values afterwards does not change the StaticEnvironment.
func NewStaticEnvironment(values map[string]string) StaticEnvironment {
	return StaticEnvironment{values: maps.Clone(values)}
}

// Value implements the Environment interface.
func (e StaticEnvironment) Value(name string) (string, error) {
	v, ok := e.values[name]
	if !ok {
		return "", ValueNotFoundError{Name: name}
	}
	return v, nil
}
