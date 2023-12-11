// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package pubsub

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	pubsubpb "cloud.google.com/go/pubsub/apiv1/pubsubpb"
	"github.com/googleapis/gax-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/z5labs/app/queue"
)

type pubsubPullClientFunc func(context.Context, *pubsubpb.PullRequest, ...gax.CallOption) (*pubsubpb.PullResponse, error)

func (f pubsubPullClientFunc) Pull(ctx context.Context, req *pubsubpb.PullRequest, opts ...gax.CallOption) (*pubsubpb.PullResponse, error) {
	return f(ctx, req, opts...)
}

func (f pubsubPullClientFunc) Acknowledge(ctx context.Context, req *pubsubpb.AcknowledgeRequest, opts ...gax.CallOption) error {
	panic("unimplemented")
}

func withClient(c pubsubClient) CommonOption {
	return commonOptionFunc(func(co *commonOptions) {
		co.pubsub = c
	})
}

func TestConsumer_Consume(t *testing.T) {
	t.Run("will return an error", func(t *testing.T) {
		t.Run("if pubsub fails to pull messages", func(t *testing.T) {
			pullErr := errors.New("failed to pull")
			client := pubsubPullClientFunc(func(ctx context.Context, pr *pubsubpb.PullRequest, co ...gax.CallOption) (*pubsubpb.PullResponse, error) {
				return nil, pullErr
			})

			c := NewConsumer(
				LogHandler(slog.Default().Handler()),
				Subscription("example"),
				MaxNumOfMessages(1),
				withClient(client),
			)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			msgs, err := c.Consume(ctx)
			if !assert.Equal(t, pullErr, err) {
				return
			}
			if !assert.Len(t, msgs, 0) {
				return
			}
		})

		t.Run("if pubsub returns zero messages", func(t *testing.T) {
			client := pubsubPullClientFunc(func(ctx context.Context, pr *pubsubpb.PullRequest, co ...gax.CallOption) (*pubsubpb.PullResponse, error) {
				resp := &pubsubpb.PullResponse{}
				return resp, nil
			})

			c := NewConsumer(
				LogHandler(slog.Default().Handler()),
				Subscription("example"),
				MaxNumOfMessages(1),
				withClient(client),
			)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			msgs, err := c.Consume(ctx)
			if !assert.Equal(t, queue.ErrNoItem, err) {
				return
			}
			if !assert.Len(t, msgs, 0) {
				return
			}
		})
	})

	t.Run("will not return an error", func(t *testing.T) {
		t.Run("if pubsub successfully retrieves messages", func(t *testing.T) {
			client := pubsubPullClientFunc(func(ctx context.Context, pr *pubsubpb.PullRequest, co ...gax.CallOption) (*pubsubpb.PullResponse, error) {
				resp := &pubsubpb.PullResponse{
					ReceivedMessages: make([]*pubsubpb.ReceivedMessage, 10),
				}
				return resp, nil
			})

			c := NewConsumer(
				LogHandler(slog.Default().Handler()),
				Subscription("example"),
				MaxNumOfMessages(1),
				withClient(client),
			)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			msgs, err := c.Consume(ctx)
			if !assert.Nil(t, err) {
				return
			}
			if !assert.Len(t, msgs, 10) {
				return
			}
		})
	})
}

type pubsubAckClientFunc func(context.Context, *pubsubpb.AcknowledgeRequest, ...gax.CallOption) error

func (f pubsubAckClientFunc) Pull(ctx context.Context, req *pubsubpb.PullRequest, opts ...gax.CallOption) (*pubsubpb.PullResponse, error) {
	panic("unimplemented")
}

func (f pubsubAckClientFunc) Acknowledge(ctx context.Context, req *pubsubpb.AcknowledgeRequest, opts ...gax.CallOption) error {
	return f(ctx, req, opts...)
}

type processorFunc func(context.Context, *pubsubpb.ReceivedMessage) error

func (f processorFunc) Process(ctx context.Context, msg *pubsubpb.ReceivedMessage) error {
	return f(ctx, msg)
}

func TestBatchAcknowledgeProcessor_Process(t *testing.T) {
	t.Run("will return an error", func(t *testing.T) {
		t.Run("if pubsub fails to batch acknowledge messages", func(t *testing.T) {
			deleteErr := errors.New("failed to delete")
			client := pubsubAckClientFunc(func(ctx context.Context, ar *pubsubpb.AcknowledgeRequest, co ...gax.CallOption) error {
				return deleteErr
			})

			var called atomic.Bool
			proc := processorFunc(func(ctx context.Context, m *pubsubpb.ReceivedMessage) error {
				called.Store(true)
				return nil
			})

			p := NewBatchAcknowledgeProcessor(
				LogHandler(slog.Default().Handler()),
				Subscription("example"),
				Processor(proc),
				withClient(client),
			)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			err := p.Process(ctx, []*pubsubpb.ReceivedMessage{{}, {}})
			if !assert.Equal(t, deleteErr, err) {
				return
			}
			if !assert.True(t, called.Load()) {
				return
			}
		})
	})

	t.Run("will not return an error", func(t *testing.T) {
		t.Run("if the inner processor fails", func(t *testing.T) {
			client := pubsubAckClientFunc(func(ctx context.Context, ar *pubsubpb.AcknowledgeRequest, co ...gax.CallOption) error {
				return nil
			})

			var called atomic.Bool
			proc := processorFunc(func(ctx context.Context, m *pubsubpb.ReceivedMessage) error {
				called.Store(true)
				return errors.New("failed")
			})

			p := NewBatchAcknowledgeProcessor(
				LogHandler(slog.Default().Handler()),
				Subscription("example"),
				Processor(proc),
				withClient(client),
			)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			err := p.Process(ctx, []*pubsubpb.ReceivedMessage{{}, {}})
			if !assert.Nil(t, err) {
				return
			}
			if !assert.True(t, called.Load()) {
				return
			}
		})

		t.Run("if pubsub successfully acknowledged all messages", func(t *testing.T) {
			client := pubsubAckClientFunc(func(ctx context.Context, ar *pubsubpb.AcknowledgeRequest, co ...gax.CallOption) error {
				return nil
			})

			var called atomic.Bool
			proc := processorFunc(func(ctx context.Context, m *pubsubpb.ReceivedMessage) error {
				called.Store(true)
				return nil
			})

			p := NewBatchAcknowledgeProcessor(
				LogHandler(slog.Default().Handler()),
				Subscription("example"),
				Processor(proc),
				withClient(client),
			)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			err := p.Process(ctx, []*pubsubpb.ReceivedMessage{{}, {}})
			if !assert.Nil(t, err) {
				return
			}
			if !assert.True(t, called.Load()) {
				return
			}
		})
	})
}
