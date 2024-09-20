// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package bedrock

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"

	"github.com/z5labs/bedrock/pkg/config"

	"github.com/stretchr/testify/assert"
)

func TestRecover(t *testing.T) {
	t.Run("will catch panic", func(t *testing.T) {
		t.Run("if panic is called with a nil argument", func(t *testing.T) {
			f := func() (err error) {
				defer Recover(&err)

				panic(nil)
				return nil
			}

			err := f()

			var perr PanicError
			if !assert.ErrorAs(t, err, &perr) {
				return
			}
			if !assert.NotEmpty(t, perr.Error()) {
				return
			}

			var rerr *runtime.PanicNilError
			if !assert.ErrorAs(t, perr, &rerr) {
				return
			}
		})

		t.Run("if panic is called with a non-error type", func(t *testing.T) {
			f := func() (err error) {
				defer Recover(&err)

				panic("hello world")
				return nil
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

		t.Run("if panic is called with a error type", func(t *testing.T) {
			panicErr := errors.New("everybody panic!")
			f := func() (err error) {
				defer Recover(&err)

				panic(panicErr)
				return nil
			}

			err := f()

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

		t.Run("even if the reference error already holds a value", func(t *testing.T) {
			prePanicErr := errors.New("everybody panic!")
			f := func() (err error) {
				defer Recover(&err)

				err = prePanicErr
				panic("hello world")
				return nil
			}

			err := f()

			// Previous error value should not be lost
			if !assert.ErrorIs(t, err, prePanicErr) {
				return
			}

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
	})
}

type configSourceFunc func(config.Store) error

func (f configSourceFunc) Apply(store config.Store) error {
	return f(store)
}

var unmarshalErr = errors.New("failed to unmarshal")

type UnmarshalTextFailure struct{}

func (x UnmarshalTextFailure) UnmarshalText(b []byte) error {
	return unmarshalErr
}

func TestRun(t *testing.T) {
	t.Run("will return an error", func(t *testing.T) {
		t.Run("if the config.Source(s) fail to be read", func(t *testing.T) {
			srcErr := errors.New("failed to apply config")
			src := configSourceFunc(func(s config.Store) error {
				return srcErr
			})

			type myConfig struct{}
			b := AppBuilderFunc[myConfig](func(ctx context.Context, cfg myConfig) (App, error) {
				return nil, nil
			})

			err := Run(context.Background(), b, src)

			var ierr ConfigReadError
			if !assert.ErrorAs(t, err, &ierr) {
				return
			}
			if !assert.NotEmpty(t, ierr.Error()) {
				return
			}
			if !assert.ErrorIs(t, ierr, srcErr) {
				return
			}
		})

		t.Run("if the config.Manager fails to unmarshal the custom config", func(t *testing.T) {
			type myConfig struct {
				Value UnmarshalTextFailure `config:"value"`
			}
			b := AppBuilderFunc[myConfig](func(ctx context.Context, cfg myConfig) (App, error) {
				return nil, nil
			})

			err := Run(context.Background(), b, config.FromYaml(strings.NewReader(`value: hello`)))

			var ierr ConfigUnmarshalError
			if !assert.ErrorAs(t, err, &ierr) {
				return
			}
			if !assert.NotEmpty(t, ierr.Error()) {
				return
			}
			if !assert.ErrorIs(t, ierr, unmarshalErr) {
				return
			}
		})

		t.Run("if the AppBuilder fails to build the App", func(t *testing.T) {
			type myConfig struct {
				Value string `config:"value"`
			}

			buildErr := errors.New("failed to build")
			b := AppBuilderFunc[myConfig](func(ctx context.Context, cfg myConfig) (App, error) {
				return nil, buildErr
			})

			err := Run(context.Background(), b, config.FromYaml(strings.NewReader(`value: hello`)))

			var ierr AppBuildError
			if !assert.ErrorAs(t, err, &ierr) {
				return
			}
			if !assert.NotEmpty(t, ierr.Error()) {
				return
			}
			if !assert.ErrorIs(t, ierr, buildErr) {
				return
			}
		})

		t.Run("if the App fails to run", func(t *testing.T) {
			type myConfig struct {
				Value string `config:"value"`
			}

			runErr := errors.New("failed to build")
			b := AppBuilderFunc[myConfig](func(ctx context.Context, cfg myConfig) (App, error) {
				app := appFunc(func(ctx context.Context) error {
					return runErr
				})
				return app, nil
			})

			err := Run(context.Background(), b, config.FromYaml(strings.NewReader(`value: hello`)))

			var ierr AppRunError
			if !assert.ErrorAs(t, err, &ierr) {
				return
			}
			if !assert.NotEmpty(t, ierr.Error()) {
				return
			}
			if !assert.ErrorIs(t, ierr, runErr) {
				return
			}
		})
	})
}
