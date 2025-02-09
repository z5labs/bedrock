// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package app

import (
	"context"
	"errors"
	"testing"

	"github.com/z5labs/bedrock/internal/try"

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

			var perr try.PanicError
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
			app := InterruptOn(runFunc(func(ctx context.Context) error {
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
