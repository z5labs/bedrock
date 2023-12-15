// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package sqs

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/z5labs/bedrock/queue"

	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/stretchr/testify/assert"
)

func withSqsClient(c sqsClient) CommonOption {
	return commonOptionFunc(func(co *commonOptions) {
		co.sqs = c
	})
}

type sqsReceiveClientFunc func(context.Context, *sqs.ReceiveMessageInput, ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error)

func (f sqsReceiveClientFunc) ReceiveMessage(ctx context.Context, in *sqs.ReceiveMessageInput, opts ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error) {
	return f(ctx, in, opts...)
}

func (f sqsReceiveClientFunc) DeleteMessageBatch(_ context.Context, _ *sqs.DeleteMessageBatchInput, _ ...func(*sqs.Options)) (*sqs.DeleteMessageBatchOutput, error) {
	panic("unimplemented")
}

func TestConsumer_Consume(t *testing.T) {
	t.Run("will return an error", func(t *testing.T) {
		t.Run("if sqs fails to receive messages", func(t *testing.T) {
			receiveErr := errors.New("failed to receive messages")
			client := sqsReceiveClientFunc(func(ctx context.Context, rmi *sqs.ReceiveMessageInput, f ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error) {
				return nil, receiveErr
			})

			c := NewConsumer(
				LogHandler(slog.Default().Handler()),
				QueueUrl("example"),
				MaxNumOfMessages(10),
				VisibilityTimeout(10),
				WaitTimeSeconds(10),
				withSqsClient(client),
			)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			msgs, err := c.Consume(ctx)
			if !assert.Equal(t, receiveErr, err) {
				return
			}
			if !assert.Len(t, msgs, 0) {
				return
			}
		})

		t.Run("if sqs receives no messages", func(t *testing.T) {
			client := sqsReceiveClientFunc(func(ctx context.Context, rmi *sqs.ReceiveMessageInput, f ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error) {
				resp := &sqs.ReceiveMessageOutput{}
				return resp, nil
			})

			c := NewConsumer(
				LogHandler(slog.Default().Handler()),
				QueueUrl("example"),
				withSqsClient(client),
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
		t.Run("if sqs successfully receives messages", func(t *testing.T) {
			client := sqsReceiveClientFunc(func(ctx context.Context, rmi *sqs.ReceiveMessageInput, f ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error) {
				resp := &sqs.ReceiveMessageOutput{
					Messages: make([]types.Message, 10),
				}
				return resp, nil
			})

			c := NewConsumer(
				LogHandler(slog.Default().Handler()),
				QueueUrl("example"),
				withSqsClient(client),
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

type sqsBatchDeleteClientFunc func(context.Context, *sqs.DeleteMessageBatchInput, ...func(*sqs.Options)) (*sqs.DeleteMessageBatchOutput, error)

func (f sqsBatchDeleteClientFunc) ReceiveMessage(_ context.Context, _ *sqs.ReceiveMessageInput, _ ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error) {
	panic("unimplemented")
}

func (f sqsBatchDeleteClientFunc) DeleteMessageBatch(ctx context.Context, in *sqs.DeleteMessageBatchInput, opts ...func(*sqs.Options)) (*sqs.DeleteMessageBatchOutput, error) {
	return f(ctx, in, opts...)
}

type processorFunc func(context.Context, types.Message) error

func (f processorFunc) Process(ctx context.Context, msg types.Message) error {
	return f(ctx, msg)
}

func TestBatchDeleteProcessor_Process(t *testing.T) {
	t.Run("will return an error", func(t *testing.T) {
		t.Run("if sqs fails to batch delete messages", func(t *testing.T) {
			deleteErr := errors.New("failed to delete")
			client := sqsBatchDeleteClientFunc(func(ctx context.Context, dmbi *sqs.DeleteMessageBatchInput, f ...func(*sqs.Options)) (*sqs.DeleteMessageBatchOutput, error) {
				return nil, deleteErr
			})

			var called atomic.Bool
			proc := processorFunc(func(ctx context.Context, m types.Message) error {
				called.Store(true)
				return nil
			})

			p := NewBatchDeleteProcessor(
				LogHandler(slog.Default().Handler()),
				QueueUrl("example"),
				Processor(proc),
				withSqsClient(client),
			)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			err := p.Process(ctx, make([]types.Message, 10))
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
			client := sqsBatchDeleteClientFunc(func(ctx context.Context, dmbi *sqs.DeleteMessageBatchInput, f ...func(*sqs.Options)) (*sqs.DeleteMessageBatchOutput, error) {
				return nil, nil
			})

			var called atomic.Bool
			proc := processorFunc(func(ctx context.Context, m types.Message) error {
				called.Store(true)
				return errors.New("failed")
			})

			p := NewBatchDeleteProcessor(
				LogHandler(slog.Default().Handler()),
				QueueUrl("example"),
				Processor(proc),
				withSqsClient(client),
			)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			err := p.Process(ctx, make([]types.Message, 10))
			if !assert.Nil(t, err) {
				return
			}
			if !assert.True(t, called.Load()) {
				return
			}
		})

		t.Run("if sqs failed to delete some messages", func(t *testing.T) {
			client := sqsBatchDeleteClientFunc(func(ctx context.Context, dmbi *sqs.DeleteMessageBatchInput, f ...func(*sqs.Options)) (*sqs.DeleteMessageBatchOutput, error) {
				var s string
				resp := &sqs.DeleteMessageBatchOutput{
					Failed: []types.BatchResultErrorEntry{
						{
							Id:          &s,
							Code:        &s,
							Message:     &s,
							SenderFault: false,
						},
					},
				}
				return resp, nil
			})

			var called atomic.Bool
			proc := processorFunc(func(ctx context.Context, m types.Message) error {
				called.Store(true)
				return nil
			})

			p := NewBatchDeleteProcessor(
				LogHandler(slog.Default().Handler()),
				QueueUrl("example"),
				Processor(proc),
				withSqsClient(client),
			)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			err := p.Process(ctx, make([]types.Message, 10))
			if !assert.Nil(t, err) {
				return
			}
			if !assert.True(t, called.Load()) {
				return
			}
		})

		t.Run("if sqs successfully deleted all messages", func(t *testing.T) {
			client := sqsBatchDeleteClientFunc(func(ctx context.Context, dmbi *sqs.DeleteMessageBatchInput, f ...func(*sqs.Options)) (*sqs.DeleteMessageBatchOutput, error) {
				resp := &sqs.DeleteMessageBatchOutput{}
				return resp, nil
			})

			var called atomic.Bool
			proc := processorFunc(func(ctx context.Context, m types.Message) error {
				called.Store(true)
				return nil
			})

			p := NewBatchDeleteProcessor(
				LogHandler(slog.Default().Handler()),
				QueueUrl("example"),
				Processor(proc),
				withSqsClient(client),
			)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			err := p.Process(ctx, make([]types.Message, 10))
			if !assert.Nil(t, err) {
				return
			}
			if !assert.True(t, called.Load()) {
				return
			}
		})
	})
}
