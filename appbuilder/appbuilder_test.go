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
	"github.com/z5labs/bedrock/lifecycle"

	"github.com/stretchr/testify/assert"
	"github.com/z5labs/sdk-go/try"
)

func TestRecover(t *testing.T) {
	t.Run("will return an error", func(t *testing.T) {
		t.Run("if the build context was cancelled before starting to build", func(t *testing.T) {
			builder := bedrock.AppBuilderFunc[struct{}](func(ctx context.Context, cfg struct{}) (bedrock.App, error) {
				return nil, nil
			})

			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			_, err := Recover(builder).Build(ctx, struct{}{})
			if !assert.ErrorIs(t, err, context.Canceled) {
				return
			}
		})

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

func TestLifecycleContext(t *testing.T) {
	t.Run("will return a single error", func(t *testing.T) {
		t.Run("if the build context was cancelled before starting to build", func(t *testing.T) {
			builder := bedrock.AppBuilderFunc[struct{}](func(ctx context.Context, cfg struct{}) (bedrock.App, error) {
				return nil, nil
			})

			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			_, err := LifecycleContext(builder, &lifecycle.Context{}).Build(ctx, struct{}{})
			if !assert.ErrorIs(t, err, context.Canceled) {
				return
			}
		})

		t.Run("if the given AppBuilder fails to build and no post run hook is registered", func(t *testing.T) {
			buildErr := errors.New("build failed")
			builder := bedrock.AppBuilderFunc[struct{}](func(ctx context.Context, cfg struct{}) (bedrock.App, error) {
				return nil, buildErr
			})

			_, err := LifecycleContext(builder, &lifecycle.Context{}).Build(context.Background(), struct{}{})
			if !assert.Equal(t, err, buildErr) {
				return
			}
		})

		t.Run("if the given AppBuilder fails to build and the post run hook succeeds", func(t *testing.T) {
			buildErr := errors.New("build failed")
			builder := bedrock.AppBuilderFunc[struct{}](func(ctx context.Context, cfg struct{}) (bedrock.App, error) {
				lc, ok := lifecycle.FromContext(ctx)
				if !ok {
					return nil, errors.New("expected lifecycle context in build context")
				}

				lc.OnPostRun(lifecycle.HookFunc(func(ctx context.Context) error {
					return nil
				}))

				return nil, buildErr
			})

			_, err := LifecycleContext(builder, &lifecycle.Context{}).Build(context.Background(), struct{}{})
			if !assert.Equal(t, err, buildErr) {
				return
			}
		})
	})

	t.Run("will return multiple errors", func(t *testing.T) {
		t.Run("if the given AppBuilder fails to build and the post run hook fails", func(t *testing.T) {
			buildErr := errors.New("build failed")
			hookErr := errors.New("hook failed")
			builder := bedrock.AppBuilderFunc[struct{}](func(ctx context.Context, cfg struct{}) (bedrock.App, error) {
				lc, ok := lifecycle.FromContext(ctx)
				if !ok {
					return nil, errors.New("expected lifecycle context in build context")
				}

				lc.OnPostRun(lifecycle.HookFunc(func(ctx context.Context) error {
					return hookErr
				}))

				return nil, buildErr
			})

			_, err := LifecycleContext(builder, &lifecycle.Context{}).Build(context.Background(), struct{}{})
			if !assert.ErrorIs(t, err, buildErr) {
				return
			}
			if !assert.ErrorIs(t, err, hookErr) {
				return
			}
		})
	})
}

func TestInterruptOn(t *testing.T) {
	t.Run("will propogate context cancellation", func(t *testing.T) {
		t.Run("if the parent context is cancelled", func(t *testing.T) {
			builder := InterruptOn(bedrock.AppBuilderFunc[struct{}](func(ctx context.Context, _ struct{}) (bedrock.App, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			}))

			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			_, err := builder.Build(ctx, struct{}{})
			if !assert.ErrorIs(t, err, context.Canceled) {
				return
			}
		})
	})
}
