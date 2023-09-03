// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package queue

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestRuntime_Run(t *testing.T) {
	t.Run("will return an error", func(t *testing.T) {
		t.Run("if one of the queue processors returns an error", func(t *testing.T) {
			qpErr := errors.New("qp error")
			qp := func(ctx context.Context, rt *Runtime) error {
				return qpErr
			}

			rt := NewRuntime(Logger(zap.NewExample()), queueProcessor(qp))

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			err := rt.Run(ctx)
			if !assert.Equal(t, qpErr, err) {
				return
			}
		})
	})

	t.Run("will not return an error", func(t *testing.T) {
		t.Run("if all queue processors complete", func(t *testing.T) {
			qp := func(ctx context.Context, rt *Runtime) error {
				return nil
			}

			rt := NewRuntime(Logger(zap.NewExample()), queueProcessor(qp), queueProcessor(qp))

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			err := rt.Run(ctx)
			if !assert.Nil(t, err) {
				return
			}
		})
	})
}

func TestPipe(t *testing.T) {
	t.Run("will not return an error", func(t *testing.T) {
		t.Run("if the consumer returns a non-EOI error", func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())

			consumeErr := errors.New("consumer failed")
			c := ConsumerFunc[int](func(ctx context.Context) (*Item[int], error) {
				defer cancel()
				return nil, consumeErr
			})

			var mu sync.Mutex
			var triggered bool
			p := ProcessorFunc[int](func(ctx context.Context, item int) error {
				mu.Lock()
				defer mu.Unlock()
				triggered = true
				return nil
			})

			rt := NewRuntime(Pipe[int](c, p))
			err := rt.Run(ctx)
			if !assert.Nil(t, err) {
				return
			}
			if !assert.False(t, triggered) {
				return
			}
		})

		t.Run("if the consumer returns a nil item", func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())

			c := ConsumerFunc[int](func(ctx context.Context) (*Item[int], error) {
				defer cancel()
				return nil, nil
			})

			var mu sync.Mutex
			var triggered bool
			p := ProcessorFunc[int](func(ctx context.Context, item int) error {
				mu.Lock()
				defer mu.Unlock()
				triggered = true
				return nil
			})

			rt := NewRuntime(Pipe[int](c, p))
			err := rt.Run(ctx)
			if !assert.Nil(t, err) {
				return
			}
			if !assert.False(t, triggered) {
				return
			}
		})

		t.Run("if the processor returns an error", func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())

			c := ConsumerFunc[int](func(ctx context.Context) (*Item[int], error) {
				return &Item[int]{Value: 1}, nil
			})

			var mu sync.Mutex
			var triggered bool
			processorErr := errors.New("processor failed")
			p := ProcessorFunc[int](func(ctx context.Context, item int) error {
				defer cancel()
				mu.Lock()
				defer mu.Unlock()
				triggered = true
				return processorErr
			})

			rt := NewRuntime(Pipe[int](c, p))
			err := rt.Run(ctx)
			if !assert.Nil(t, err) {
				return
			}
			if !assert.True(t, triggered) {
				return
			}
		})
	})
}

func TestSequential(t *testing.T) {
	t.Run("will not return an error", func(t *testing.T) {
		t.Run("if the consumer returns a non-EOI error", func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())

			consumeErr := errors.New("consumer failed")
			c := ConsumerFunc[int](func(ctx context.Context) (*Item[int], error) {
				defer cancel()
				return nil, consumeErr
			})

			var triggered bool
			p := ProcessorFunc[int](func(ctx context.Context, item int) error {
				triggered = true
				return nil
			})

			rt := NewRuntime(Sequential[int](c, p))
			err := rt.Run(ctx)
			if !assert.Nil(t, err) {
				return
			}
			if !assert.False(t, triggered) {
				return
			}
		})

		t.Run("if the consumer returns a nil item", func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())

			c := ConsumerFunc[int](func(ctx context.Context) (*Item[int], error) {
				defer cancel()
				return nil, nil
			})

			var triggered bool
			p := ProcessorFunc[int](func(ctx context.Context, item int) error {
				triggered = true
				return nil
			})

			rt := NewRuntime(Sequential[int](c, p))
			err := rt.Run(ctx)
			if !assert.Nil(t, err) {
				return
			}
			if !assert.False(t, triggered) {
				return
			}
		})

		t.Run("if the processor returns an error", func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())

			c := ConsumerFunc[int](func(ctx context.Context) (*Item[int], error) {
				return &Item[int]{Value: 1}, nil
			})

			var triggered bool
			processorErr := errors.New("processor failed")
			p := ProcessorFunc[int](func(ctx context.Context, item int) error {
				defer cancel()
				triggered = true
				return processorErr
			})

			rt := NewRuntime(Sequential[int](c, p))
			err := rt.Run(ctx)
			if !assert.Nil(t, err) {
				return
			}
			if !assert.True(t, triggered) {
				return
			}
		})
	})
}
