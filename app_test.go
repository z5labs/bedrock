// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package bedrock

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/z5labs/bedrock/pkg/otelconfig"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/trace"
)

type readerFunc func([]byte) (int, error)

func (f readerFunc) Read(b []byte) (int, error) {
	return f(b)
}

type otelInitFunc func() (trace.TracerProvider, error)

func (f otelInitFunc) Init() (trace.TracerProvider, error) {
	return f()
}

type runtimeFunc func(context.Context) error

func (f runtimeFunc) Run(ctx context.Context) error {
	return f(ctx)
}

func TestApp_Run(t *testing.T) {
	t.Run("will return an error", func(t *testing.T) {
		t.Run("if the config reader fails to read", func(t *testing.T) {
			readerErr := errors.New("failed to read")
			r := readerFunc(func(b []byte) (int, error) {
				return 0, readerErr
			})

			app := New(Config(r))
			err := app.Run()
			if !assert.Equal(t, readerErr, err) {
				return
			}
		})

		t.Run("if the config is not valid yaml", func(t *testing.T) {
			r := strings.NewReader(`hello world`)

			app := New(Config(io.NopCloser(r)))
			err := app.Run()
			if !assert.IsType(t, viper.ConfigParseError{}, err) {
				return
			}
		})

		t.Run("if it fails to get the otel initializer", func(t *testing.T) {
			initErr := errors.New("failed to init")
			app := New(InitTracerProvider(func(_ context.Context) (otelconfig.Initializer, error) {
				return nil, initErr
			}))

			err := app.Run()
			if !assert.Equal(t, initErr, err) {
				return
			}
		})

		t.Run("if the otel initializer fails to initialize", func(t *testing.T) {
			initErr := errors.New("failed to init")
			app := New(InitTracerProvider(func(_ context.Context) (otelconfig.Initializer, error) {
				initer := otelInitFunc(func() (trace.TracerProvider, error) {
					return nil, initErr
				})
				return initer, nil
			}))

			err := app.Run()
			if !assert.Equal(t, initErr, err) {
				return
			}
		})

		t.Run("if the runtime builder fails to build", func(t *testing.T) {
			buildErr := errors.New("failed to build")
			app := New(WithRuntimeBuilderFunc(func(_ context.Context) (Runtime, error) {
				return nil, buildErr
			}))

			err := app.Run()
			if !assert.Equal(t, buildErr, err) {
				return
			}
		})

		t.Run("if the runtime builder returns a nil runtime", func(t *testing.T) {
			app := New(WithRuntimeBuilderFunc(func(_ context.Context) (Runtime, error) {
				return nil, nil
			}))

			err := app.Run()
			if !assert.Equal(t, errNilRuntime, err) {
				return
			}
		})

		t.Run("if the runtime builder panics with a non-error", func(t *testing.T) {
			app := New(WithRuntimeBuilderFunc(func(_ context.Context) (Runtime, error) {
				panic("hello")
				return nil, nil
			}))

			err := app.Run()
			if !assert.IsType(t, panicError{}, err) {
				return
			}

			perr := err.(panicError)
			if !assert.NotEmpty(t, perr.Error()) {
				return
			}
			if !assert.Equal(t, "hello", perr.v) {
				return
			}
		})

		t.Run("if the runtime builder panics with an error", func(t *testing.T) {
			buildErr := errors.New("failed to build")
			app := New(WithRuntimeBuilderFunc(func(_ context.Context) (Runtime, error) {
				panic(buildErr)
				return nil, nil
			}))

			err := app.Run()
			if !assert.Equal(t, buildErr, err) {
				return
			}
		})

		t.Run("if the runtime run method returns an error", func(t *testing.T) {
			runErr := errors.New("failed to run")
			app := New(WithRuntimeBuilderFunc(func(_ context.Context) (Runtime, error) {
				rtFunc := runtimeFunc(func(ctx context.Context) error {
					return runErr
				})
				return rtFunc, nil
			}))

			err := app.Run()
			if !assert.Equal(t, runErr, err) {
				return
			}
		})

		t.Run("if one of the runtimes run methods returns an error", func(t *testing.T) {
			runErr := errors.New("failed to run")
			app := New(
				WithRuntimeBuilderFunc(func(_ context.Context) (Runtime, error) {
					rtFunc := runtimeFunc(func(ctx context.Context) error {
						return runErr
					})
					return rtFunc, nil
				}),
				WithRuntimeBuilderFunc(func(_ context.Context) (Runtime, error) {
					rtFunc := runtimeFunc(func(ctx context.Context) error {
						<-ctx.Done()
						return nil
					})
					return rtFunc, nil
				}),
			)

			err := app.Run()
			if !assert.Equal(t, runErr, err) {
				return
			}
		})

		t.Run("if the runtime run method panics", func(t *testing.T) {
			runErr := errors.New("failed to run")
			app := New(WithRuntimeBuilderFunc(func(_ context.Context) (Runtime, error) {
				rtFunc := runtimeFunc(func(ctx context.Context) error {
					panic(runErr)
					return nil
				})
				return rtFunc, nil
			}))

			err := app.Run()
			if !assert.Equal(t, runErr, err) {
				return
			}
		})

		t.Run("if one of the runtimes run methods panics", func(t *testing.T) {
			runErr := errors.New("failed to run")
			app := New(
				WithRuntimeBuilderFunc(func(_ context.Context) (Runtime, error) {
					rtFunc := runtimeFunc(func(ctx context.Context) error {
						panic(runErr)
						return nil
					})
					return rtFunc, nil
				}),
				WithRuntimeBuilderFunc(func(_ context.Context) (Runtime, error) {
					rtFunc := runtimeFunc(func(ctx context.Context) error {
						<-ctx.Done()
						return nil
					})
					return rtFunc, nil
				}),
			)

			err := app.Run()
			if !assert.Equal(t, runErr, err) {
				return
			}
		})

		t.Run("if a finalizer returns an error", func(t *testing.T) {
			finalizeErr := errors.New("failed to finalize")
			app := New(
				WithRuntimeBuilderFunc(func(ctx context.Context) (Runtime, error) {
					fs := FinalizersFromContext(ctx)
					fs.Add(func() error {
						return finalizeErr
					})

					rt := runtimeFunc(func(ctx context.Context) error {
						return nil
					})
					return rt, nil
				}),
			)

			err := app.Run()
			if !assert.IsType(t, multiError{}, err) {
				return
			}

			merr := err.(multiError)
			if !assert.NotEmpty(t, merr.Error()) {
				return
			}
			if !assert.Len(t, merr.errors, 1) {
				return
			}
			if !assert.Equal(t, finalizeErr, merr.errors[0]) {
				return
			}
		})
	})
}
