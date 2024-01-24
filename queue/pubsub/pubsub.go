// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package pubsub

import (
	"context"
	"log/slog"

	"github.com/z5labs/bedrock/pkg/noop"
	"github.com/z5labs/bedrock/pkg/slogfield"
	"github.com/z5labs/bedrock/queue"
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

// ConsumerOption are options for configuring the Consumer.
type ConsumerOption interface {
	applyConsumer(*consumerOptions)
}

type consumerOptionFunc func(*consumerOptions)

func (f consumerOptionFunc) applyConsumer(co *consumerOptions) {
	f(co)
}

// MaxNumOfMessages defines the maximum number of messages which
// Google Cloud PubSub will return in a single response.
//
// PubSub never returns more messages than this value (however,
// fewer messages might be returned). Must be a positive integer.
func MaxNumOfMessages(n int32) ConsumerOption {
	return consumerOptionFunc(func(co *consumerOptions) {
		co.maxNumOfMessages = n
	})
}

// Consumer consumes messages from Google Cloud PubSub.
type Consumer struct {
	log    *slog.Logger
	pubsub pubsubPullClient

	subscription     string
	maxNumOfMessages int32
}

// NewConsumer returns a fully initialized Consumer.
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
		log:              slog.New(co.logHandler),
		pubsub:           co.pubsub,
		subscription:     co.subscription,
		maxNumOfMessages: co.maxNumOfMessages,
	}
}

// Consume implements the queue.Consumer interface.
//
// A PullRequest is sent to Google Cloud PubSub with
// the configured options (e.g. max number of messages, etc.).
// An error is only returned in the case where the PubSub request
// fails or PubSub returns zero messages. In the case of zero messages,
// the error, queue.ErrNoItem, is returned which allows the queue based
// runtimes to disregard this as a failure and retry consuming messages.
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

// BatchAcknowledgeProcessorOption are options for configuring the BatchAknowledgeProcessor.
type BatchAcknowledgeProcessorOption interface {
	applyProcessor(*batchAcknowledgeProcessorOptions)
}

type batchAckProcessorOptionFunc func(*batchAcknowledgeProcessorOptions)

func (f batchAckProcessorOptionFunc) applyProcessor(bo *batchAcknowledgeProcessorOptions) {
	f(bo)
}

// Processor configures the underlying single message queue.Processor
// which the BatchAcknowledgeProcessor calls when concurrently processing a
// batch of messages.
func Processor(p queue.Processor[*pubsubpb.ReceivedMessage]) BatchAcknowledgeProcessorOption {
	return batchAckProcessorOptionFunc(func(bo *batchAcknowledgeProcessorOptions) {
		bo.inner = p
	})
}

// BatchAcknowledgeProcessor concurrently processes and acknowledges PubSub messages.
type BatchAcknowledgeProcessor struct {
	log    *slog.Logger
	pubsub pubsubAckClient

	subscription string
	inner        queue.Processor[*pubsubpb.ReceivedMessage]
}

// NewBatchAcknowledgeProcessor returns a fully initialized BatchAcknowledgeProcessor.
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
		log:          slog.New(bo.logHandler),
		pubsub:       bo.pubsub,
		subscription: bo.subscription,
		inner:        bo.inner,
	}
}

// Process implements the queue.Processor interface.
//
// Each message is processed concurrently using the processor
// that was provided to the BatchAcknowledgeProcessor when it was
// created. If the inner processor returns an error for a message,
// it will not be acknowledged in PubSub and will be reprocessed after
// the VisibilityTimeout expires. If no error is returned, the
// message will be collected with the other messages from the slice,
// msgs, to be acknowledged together in a single Acknowledge request to PubSub.
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
