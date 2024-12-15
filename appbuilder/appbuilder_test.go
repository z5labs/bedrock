// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package appbuilder

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
			buildErr := errors.New("failed to build")
			builder := Recover(bedrock.AppBuilderFunc[struct{}](func(ctx context.Context, cfg struct{}) (bedrock.App, error) {
				return nil, buildErr
			}))

			_, err := builder.Build(context.Background(), struct{}{})
			if !assert.Equal(t, buildErr, err) {
				return
			}
		})

		t.Run("if the underlying App panics with an error value", func(t *testing.T) {
			buildErr := errors.New("failed to build")
			builder := Recover(bedrock.AppBuilderFunc[struct{}](func(ctx context.Context, cfg struct{}) (bedrock.App, error) {
				panic(buildErr)
				return nil, nil
			}))

			_, err := builder.Build(context.Background(), struct{}{})
			if !assert.ErrorIs(t, err, buildErr) {
				return
			}
		})

		t.Run("if the underlying App panics with a non-error value", func(t *testing.T) {
			builder := Recover(bedrock.AppBuilderFunc[struct{}](func(ctx context.Context, cfg struct{}) (bedrock.App, error) {
				panic("hello world")
				return nil, nil
			}))

			_, err := builder.Build(context.Background(), struct{}{})

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
