// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package app

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

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
		t.Run("if Lifecycle.PreRun fails", func(t *testing.T) {
			base := runFunc(func(ctx context.Context) error {
				return nil
			})

			preRunErr := errors.New("failed to pre run")
			app := WithLifecycleHooks(base, Lifecycle{
				PreRun: LifecycleHookFunc(func(ctx context.Context) error {
					return preRunErr
				}),
			})

			err := app.Run(context.Background())
			if !assert.ErrorIs(t, err, preRunErr) {
				return
			}
		})

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
	})

	t.Run("will not return an error", func(t *testing.T) {
		t.Run("if the Lifecycle.PostRun fails", func(t *testing.T) {
			base := runFunc(func(ctx context.Context) error {
				return nil
			})

			app := WithLifecycleHooks(base, Lifecycle{
				PostRun: LifecycleHookFunc(func(ctx context.Context) error {
					return errors.New("failed to post run")
				}),
			})

			err := app.Run(context.Background())
			if !assert.Nil(t, err) {
				return
			}
		})
	})
}
