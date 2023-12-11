// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package pubsub provides default implementations for using Google Cloud PubSub with the runtimes in the queue package.
package pubsub

import (
	"context"
	"log/slog"

	"github.com/z5labs/app/pkg/noop"
	"github.com/z5labs/app/pkg/otelslog"
	"github.com/z5labs/app/pkg/slogfield"
	"github.com/z5labs/app/queue"
	"golang.org/x/sync/errgroup"

	pubsubpb "cloud.google.com/go/pubsub/apiv1/pubsubpb"
	"github.com/googleapis/gax-go/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type pubsubPullClient interface {
	Pull(context.Context, *pubsubpb.PullRequest, ...gax.CallOption) (*pubsubpb.PullResponse, error)
}

type consumerOptions struct {
	commonOptions

	maxNumOfMessages int32
}

// ConsumerOption
type ConsumerOption interface {
	applyConsumer(*consumerOptions)
}

type consumerOptionFunc func(*consumerOptions)

func (f consumerOptionFunc) applyConsumer(co *consumerOptions) {
	f(co)
}

// MaxNumOfMessages
func MaxNumOfMessages(n int32) ConsumerOption {
	return consumerOptionFunc(func(co *consumerOptions) {
		co.maxNumOfMessages = n
	})
}

// Consumer
type Consumer struct {
	log    *slog.Logger
	pubsub pubsubPullClient

	subscription     string
	maxNumOfMessages int32
}

// NewConsumer
func NewConsumer(opts ...ConsumerOption) *Consumer {
	co := &consumerOptions{
		commonOptions: commonOptions{
			logHandler: noop.LogHandler{},
		},
	}
	for _, opt := range opts {
		opt.applyConsumer(co)
	}
	return &Consumer{
		log:              otelslog.New(co.logHandler),
		pubsub:           co.pubsub,
		subscription:     co.subscription,
		maxNumOfMessages: co.maxNumOfMessages,
	}
}

// Consume implements the queue.Consumer interface.
func (c *Consumer) Consume(ctx context.Context) ([]*pubsubpb.ReceivedMessage, error) {
	spanCtx, span := otel.Tracer("pubsub").Start(ctx, "Consumer.Consume")
	defer span.End()

	resp, err := c.pubsub.Pull(spanCtx, &pubsubpb.PullRequest{
		Subscription: c.subscription,
		MaxMessages:  c.maxNumOfMessages,
	})
	if err != nil {
		c.log.ErrorContext(spanCtx, "failed to pull pubsub for messages", slogfield.Error(err))
		return nil, err
	}
	c.log.InfoContext(spanCtx, "received messages", slogfield.Int("num_of_messages", len(resp.ReceivedMessages)))
	if len(resp.ReceivedMessages) == 0 {
		return nil, queue.ErrNoItem
	}
	return resp.ReceivedMessages, nil
}

type pubsubAckClient interface {
	Acknowledge(context.Context, *pubsubpb.AcknowledgeRequest, ...gax.CallOption) error
}

type batchAcknowledgeProcessorOptions struct {
	commonOptions

	inner queue.Processor[*pubsubpb.ReceivedMessage]
}

// BatchAcknowledgeProcessorOption
type BatchAcknowledgeProcessorOption interface {
	applyProcessor(*batchAcknowledgeProcessorOptions)
}

type batchAckProcessorOptionFunc func(*batchAcknowledgeProcessorOptions)

func (f batchAckProcessorOptionFunc) applyProcessor(bo *batchAcknowledgeProcessorOptions) {
	f(bo)
}

// Processor
func Processor(p queue.Processor[*pubsubpb.ReceivedMessage]) BatchAcknowledgeProcessorOption {
	return batchAckProcessorOptionFunc(func(bo *batchAcknowledgeProcessorOptions) {
		bo.inner = p
	})
}

// BatchAcknowledgeProcessor
type BatchAcknowledgeProcessor struct {
	log    *slog.Logger
	pubsub pubsubAckClient

	subscription string
	inner        queue.Processor[*pubsubpb.ReceivedMessage]
}

func NewBatchAcknowledgeProcessor(opts ...BatchAcknowledgeProcessorOption) *BatchAcknowledgeProcessor {
	bo := &batchAcknowledgeProcessorOptions{
		commonOptions: commonOptions{
			logHandler: noop.LogHandler{},
		},
	}
	for _, opt := range opts {
		opt.applyProcessor(bo)
	}
	return &BatchAcknowledgeProcessor{
		log:          otelslog.New(bo.logHandler),
		pubsub:       bo.pubsub,
		subscription: bo.subscription,
		inner:        bo.inner,
	}
}

// Process implements the queue.Processor interface.
func (p *BatchAcknowledgeProcessor) Process(ctx context.Context, msgs []*pubsubpb.ReceivedMessage) error {
	spanCtx, span := otel.Tracer("pubsub").Start(ctx, "BatchAcknowledgeProcessor.Process", trace.WithAttributes(
		attribute.Int("num_of_messages", len(msgs)),
	))
	defer span.End()

	msgCh := make(chan *pubsubpb.ReceivedMessage)
	g, gctx := errgroup.WithContext(spanCtx)
	for _, msg := range msgs {
		msg := msg
		g.Go(func() error {
			err := p.inner.Process(gctx, msg)
			if err != nil {
				p.log.ErrorContext(gctx, "failed to process message", slogfield.Error(err))
				return nil
			}
			msgCh <- msg
			return nil
		})
	}

	g2, _ := errgroup.WithContext(spanCtx)
	g2.Go(func() error {
		defer close(msgCh)
		return g.Wait()
	})

	ackIds := make([]string, 0, len(msgs))
	g2.Go(func() error {
		for msg := range msgCh {
			if msg == nil {
				return nil
			}
			ackIds = append(ackIds, msg.AckId)
		}
		return nil
	})

	// Always try to acknowledge messages even if
	// context has been cancelled.
	_ = g2.Wait()
	if len(ackIds) == 0 {
		return nil
	}

	err := p.pubsub.Acknowledge(spanCtx, &pubsubpb.AcknowledgeRequest{
		Subscription: p.subscription,
		AckIds:       ackIds,
	})
	if err != nil {
		p.log.ErrorContext(
			spanCtx,
			"failed to batch acknowledge messages",
			slogfield.Int("num_of_delete_entries", len(ackIds)),
			slogfield.Error(err),
		)
		return err
	}
	return nil
}
