// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package queue

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSequentialRuntime_Run(t *testing.T) {
	t.Run("will stop", func(t *testing.T) {
		t.Run("if the context is cancelled before consuming", func(t *testing.T) {
			c := consumerFunc[int](func(ctx context.Context) (int, error) {
				return 0, nil
			})
			p := processorFunc[int](func(ctx context.Context, i int) error {
				return nil
			})

			rt := Sequential[int](c, p)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			cancel()
			err := rt.Run(ctx)
			if !assert.Nil(t, err) {
				return
			}
		})

		t.Run("if the context is cancelled before processing", func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			c := consumerFunc[int](func(ctx context.Context) (int, error) {
				cancel()
				return 0, nil
			})
			p := processorFunc[int](func(ctx context.Context, i int) error {
				return nil
			})

			rt := Sequential[int](c, p)

			err := rt.Run(ctx)
			if !assert.Nil(t, err) {
				return
			}
		})
	})

	t.Run("will continue", func(t *testing.T) {
		t.Run("if it fails to consume", func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			var count atomic.Uint64
			c := consumerFunc[int](func(ctx context.Context) (int, error) {
				count.Add(1)
				if count.Load() > 5 {
					cancel()
				}
				return 0, errors.New("failed to consume")
			})

			called := false
			p := processorFunc[int](func(ctx context.Context, i int) error {
				called = true
				return nil
			})

			rt := Sequential[int](c, p)

			err := rt.Run(ctx)
			if !assert.Nil(t, err) {
				return
			}
			if !assert.False(t, called) {
				return
			}
		})

		t.Run("if it fails to process", func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			c := consumerFunc[int](func(ctx context.Context) (int, error) {
				return 0, nil
			})

			var count atomic.Uint64
			p := processorFunc[int](func(ctx context.Context, i int) error {
				count.Add(1)
				if count.Load() > 5 {
					cancel()
				}
				return errors.New("failed to process")
			})

			rt := Sequential[int](c, p)

			err := rt.Run(ctx)
			if !assert.Nil(t, err) {
				return
			}
			if !assert.Greater(t, count.Load(), uint64(1)) {
				return
			}
		})
	})
}

func TestConcurrentRuntime_Run(t *testing.T) {
	t.Run("will stop", func(t *testing.T) {
		t.Run("if the context is cancelled before consuming", func(t *testing.T) {
			c := consumerFunc[int](func(ctx context.Context) (int, error) {
				return 0, nil
			})
			p := processorFunc[int](func(ctx context.Context, i int) error {
				return nil
			})

			rt := Concurrent[int](c, p)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			cancel()
			err := rt.Run(ctx)
			if !assert.Nil(t, err) {
				return
			}
		})

		t.Run("if the context is cancelled before processing", func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			c := consumerFunc[int](func(ctx context.Context) (int, error) {
				cancel()
				return 0, nil
			})
			p := processorFunc[int](func(ctx context.Context, i int) error {
				return nil
			})

			rt := Concurrent[int](c, p)

			err := rt.Run(ctx)
			if !assert.Nil(t, err) {
				return
			}
		})
	})

	t.Run("will continue", func(t *testing.T) {
		t.Run("if it fails to consume", func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			var count atomic.Uint64
			c := consumerFunc[int](func(ctx context.Context) (int, error) {
				count.Add(1)
				if count.Load() > 5 {
					cancel()
				}
				return 0, errors.New("failed to consume")
			})

			called := false
			p := processorFunc[int](func(ctx context.Context, i int) error {
				called = true
				return nil
			})

			rt := Concurrent[int](c, p)

			err := rt.Run(ctx)
			if !assert.Nil(t, err) {
				return
			}
			if !assert.False(t, called) {
				return
			}
		})

		t.Run("if it panics while consuming with a non-error", func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			var count atomic.Uint64
			c := consumerFunc[int](func(ctx context.Context) (int, error) {
				count.Add(1)
				if count.Load() > 5 {
					cancel()
				}
				panic("panic while consuming")
				return 0, nil
			})

			called := false
			p := processorFunc[int](func(ctx context.Context, i int) error {
				called = true
				return nil
			})

			rt := Concurrent[int](c, p)

			err := rt.Run(ctx)
			if !assert.Nil(t, err) {
				return
			}
			if !assert.False(t, called) {
				return
			}
		})

		t.Run("if it panics while consuming with a error", func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			var count atomic.Uint64
			c := consumerFunc[int](func(ctx context.Context) (int, error) {
				count.Add(1)
				if count.Load() > 5 {
					cancel()
				}
				panic(errors.New("panic while consuming"))
				return 0, nil
			})

			called := false
			p := processorFunc[int](func(ctx context.Context, i int) error {
				called = true
				return nil
			})

			rt := Concurrent[int](c, p)

			err := rt.Run(ctx)
			if !assert.Nil(t, err) {
				return
			}
			if !assert.False(t, called) {
				return
			}
		})

		t.Run("if it fails to process", func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			c := consumerFunc[int](func(ctx context.Context) (int, error) {
				return 0, nil
			})

			var count atomic.Uint64
			p := processorFunc[int](func(ctx context.Context, i int) error {
				count.Add(1)
				if count.Load() > 5 {
					cancel()
				}
				return errors.New("failed to process")
			})

			rt := Concurrent[int](c, p)

			err := rt.Run(ctx)
			if !assert.Nil(t, err) {
				return
			}
			if !assert.Greater(t, count.Load(), uint64(1)) {
				return
			}
		})

		t.Run("if it panics while processing with a non-error", func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			c := consumerFunc[int](func(ctx context.Context) (int, error) {
				return 0, nil
			})

			var count atomic.Uint64
			p := processorFunc[int](func(ctx context.Context, i int) error {
				count.Add(1)
				if count.Load() > 5 {
					cancel()
				}
				panic("panic while processing")
				return nil
			})

			rt := Concurrent[int](c, p)

			err := rt.Run(ctx)
			if !assert.Nil(t, err) {
				return
			}
			if !assert.Greater(t, count.Load(), uint64(1)) {
				return
			}
		})

		t.Run("if it panics while processing with a error", func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			c := consumerFunc[int](func(ctx context.Context) (int, error) {
				return 0, nil
			})

			var count atomic.Uint64
			p := processorFunc[int](func(ctx context.Context, i int) error {
				count.Add(1)
				if count.Load() > 5 {
					cancel()
				}
				panic(errors.New("panic while processing"))
				return nil
			})

			rt := Concurrent[int](c, p)

			err := rt.Run(ctx)
			if !assert.Nil(t, err) {
				return
			}
			if !assert.Greater(t, count.Load(), uint64(1)) {
				return
			}
		})
	})
}
