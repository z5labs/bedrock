// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package app

import (
	"context"
	"errors"
	"testing"

	"github.com/z5labs/bedrock"

	"github.com/stretchr/testify/assert"
)

func TestRecover(t *testing.T) {
	t.Run("will return an error", func(t *testing.T) {
		t.Run("if the underlying App returns an error", func(t *testing.T) {
			appErr := errors.New("failed to run")
			app := Recover(runFunc(func(ctx context.Context) error {
				return appErr
			}))

			err := app.Run(context.Background())
			if !assert.Equal(t, appErr, err) {
				return
			}
		})

		t.Run("if the underlying App panics with an error value", func(t *testing.T) {
			appErr := errors.New("failed to run")
			app := Recover(runFunc(func(ctx context.Context) error {
				panic(appErr)
				return nil
			}))

			err := app.Run(context.Background())
			if !assert.ErrorIs(t, err, appErr) {
				return
			}
		})

		t.Run("if the underlying App panics with a non-error value", func(t *testing.T) {
			app := Recover(runFunc(func(ctx context.Context) error {
				panic("hello world")
				return nil
			}))

			err := app.Run(context.Background())

			var perr bedrock.PanicError
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

func TestWithSignalNotifications(t *testing.T) {
	t.Run("will propogate context cancellation", func(t *testing.T) {
		t.Run("if the parent context is cancelled", func(t *testing.T) {
			app := WithSignalNotifications(runFunc(func(ctx context.Context) error {
				<-ctx.Done()
				return ctx.Err()
			}))

			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			err := app.Run(ctx)
			if !assert.ErrorIs(t, err, context.Canceled) {
				return
			}
		})
	})
}

func TestWithLifecycleHooks(t *testing.T) {
	t.Run("will return error", func(t *testing.T) {
		t.Run("if the underlying app fails", func(t *testing.T) {
			baseErr := errors.New("failed to run app")
			base := runFunc(func(ctx context.Context) error {
				return baseErr
			})

			app := WithLifecycleHooks(base, Lifecycle{})

			err := app.Run(context.Background())
			if !assert.ErrorIs(t, err, baseErr) {
				return
			}
		})

		t.Run("if the Lifecycle.PostRun hook fails", func(t *testing.T) {
			base := runFunc(func(ctx context.Context) error {
				return nil
			})

			postRunErr := errors.New("failed to post run")
			postRun := LifecycleHookFunc(func(ctx context.Context) error {
				return postRunErr
			})

			app := WithLifecycleHooks(base, Lifecycle{
				PostRun: postRun,
			})

			err := app.Run(context.Background())
			if !assert.ErrorIs(t, err, postRunErr) {
				return
			}
		})

		t.Run("if both underlying app and the Lifecycle.PostRun hook fail", func(t *testing.T) {
			baseErr := errors.New("failed to run app")
			base := runFunc(func(ctx context.Context) error {
				return baseErr
			})

			postRunErr := errors.New("failed to post run")
			postRun := LifecycleHookFunc(func(ctx context.Context) error {
				return postRunErr
			})

			app := WithLifecycleHooks(base, Lifecycle{
				PostRun: postRun,
			})

			err := app.Run(context.Background())
			if !assert.ErrorIs(t, err, baseErr) {
				return
			}
			if !assert.ErrorIs(t, err, postRunErr) {
				return
			}
		})
	})

	t.Run("will not return an error", func(t *testing.T) {
		t.Run("if both the underlying app and the Lifecycle.PostRun do not fail", func(t *testing.T) {
			base := runFunc(func(ctx context.Context) error {
				return nil
			})

			postRun := LifecycleHookFunc(func(ctx context.Context) error {
				return nil
			})

			app := WithLifecycleHooks(base, Lifecycle{
				PostRun: postRun,
			})

			err := app.Run(context.Background())
			if !assert.Nil(t, err) {
				return
			}
		})
	})
}

func TestComposeLifecycleHooks(t *testing.T) {
	t.Run("will return an error", func(t *testing.T) {
		t.Run("if a single lifecycle hook failed", func(t *testing.T) {
			errHookFailed := errors.New("failed to run hook")

			hook := ComposeLifecycleHooks(
				LifecycleHookFunc(func(ctx context.Context) error {
					return nil
				}),
				LifecycleHookFunc(func(ctx context.Context) error {
					return errHookFailed
				}),
				LifecycleHookFunc(func(ctx context.Context) error {
					return nil
				}),
			)

			err := hook.Run(context.Background())
			if !assert.ErrorIs(t, err, errHookFailed) {
				return
			}
		})

		t.Run("if multiple lifecycle hooks failed", func(t *testing.T) {
			errHookFailedOne := errors.New("failed to run hook: one")
			errHookFailedTwo := errors.New("failed to run hook: two")
			errHookFailedThree := errors.New("failed to run hook: three")

			hook := ComposeLifecycleHooks(
				LifecycleHookFunc(func(ctx context.Context) error {
					return errHookFailedOne
				}),
				LifecycleHookFunc(func(ctx context.Context) error {
					return errHookFailedTwo
				}),
				LifecycleHookFunc(func(ctx context.Context) error {
					return errHookFailedThree
				}),
			)

			err := hook.Run(context.Background())
			if !assert.ErrorIs(t, err, errHookFailedOne) {
				return
			}
			if !assert.ErrorIs(t, err, errHookFailedTwo) {
				return
			}
			if !assert.ErrorIs(t, err, errHookFailedThree) {
				return
			}
		})
	})
}
