// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package sqs provides default implementations for using AWS SQS with the runtimes in the queue package.
package sqs

import (
	"context"
	"log/slog"

	"github.com/z5labs/bedrock/pkg/noop"
	"github.com/z5labs/bedrock/pkg/otelslog"
	"github.com/z5labs/bedrock/pkg/slogfield"
	"github.com/z5labs/bedrock/queue"

	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"
)

type consumerOptions struct {
	commonOptions

	maxNumOfMessages  int32
	visibilityTimeout int32
	waitTimeSeconds   int32
}

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

// VisibilityTimeout
func VisibilityTimeout(n int32) ConsumerOption {
	return consumerOptionFunc(func(co *consumerOptions) {
		co.visibilityTimeout = n
	})
}

// WaitTimeSeconds
func WaitTimeSeconds(n int32) ConsumerOption {
	return consumerOptionFunc(func(co *consumerOptions) {
		co.waitTimeSeconds = n
	})
}

type sqsReceiveClient interface {
	ReceiveMessage(context.Context, *sqs.ReceiveMessageInput, ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error)
}

// Consumer
type Consumer struct {
	log *slog.Logger
	sqs sqsReceiveClient

	queueUrl          string
	maxNumOfMessages  int32
	visibilityTimeout int32
	waitTimeSeconds   int32
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
		log:               otelslog.New(co.logHandler),
		sqs:               co.sqs,
		queueUrl:          co.queueUrl,
		maxNumOfMessages:  co.maxNumOfMessages,
		visibilityTimeout: co.visibilityTimeout,
		waitTimeSeconds:   co.waitTimeSeconds,
	}
}

// Consume
func (c *Consumer) Consume(ctx context.Context) ([]types.Message, error) {
	spanCtx, span := otel.Tracer("sqs").Start(ctx, "Consumer.Consume")
	defer span.End()

	resp, err := c.sqs.ReceiveMessage(spanCtx, &sqs.ReceiveMessageInput{
		QueueUrl:            &c.queueUrl,
		MaxNumberOfMessages: c.maxNumOfMessages,
		VisibilityTimeout:   c.visibilityTimeout,
		WaitTimeSeconds:     c.waitTimeSeconds,
	})
	if err != nil {
		c.log.ErrorContext(spanCtx, "failed to receive messages", slogfield.Error(err))
		return nil, err
	}

	c.log.InfoContext(spanCtx, "received messages", slogfield.Int("num_of_messages", len(resp.Messages)))
	if len(resp.Messages) == 0 {
		return nil, queue.ErrNoItem
	}
	return resp.Messages, nil
}

type batchDeleteProcessorOptions struct {
	commonOptions

	inner queue.Processor[types.Message]
}

// BatchDeleteProcessorOption
type BatchDeleteProcessorOption interface {
	applyProcessor(*batchDeleteProcessorOptions)
}

type batchDeleteProcessorOptionFunc func(*batchDeleteProcessorOptions)

func (f batchDeleteProcessorOptionFunc) applyProcessor(bo *batchDeleteProcessorOptions) {
	f(bo)
}

// Processor
func Processor(p queue.Processor[types.Message]) BatchDeleteProcessorOption {
	return batchDeleteProcessorOptionFunc(func(bo *batchDeleteProcessorOptions) {
		bo.inner = p
	})
}

type sqsBatchDeleteClient interface {
	DeleteMessageBatch(context.Context, *sqs.DeleteMessageBatchInput, ...func(*sqs.Options)) (*sqs.DeleteMessageBatchOutput, error)
}

// BatchDeleteProcessor
type BatchDeleteProcessor struct {
	log *slog.Logger
	sqs sqsBatchDeleteClient

	queueUrl string
	inner    queue.Processor[types.Message]
}

// NewBatchDeleteProcessor
func NewBatchDeleteProcessor(opts ...BatchDeleteProcessorOption) *BatchDeleteProcessor {
	bo := &batchDeleteProcessorOptions{
		commonOptions: commonOptions{
			logHandler: noop.LogHandler{},
		},
	}
	for _, opt := range opts {
		opt.applyProcessor(bo)
	}
	return &BatchDeleteProcessor{
		log:      otelslog.New(bo.logHandler),
		sqs:      bo.sqs,
		queueUrl: bo.queueUrl,
		inner:    bo.inner,
	}
}

// Process
func (p *BatchDeleteProcessor) Process(ctx context.Context, msgs []types.Message) error {
	spanCtx, span := otel.Tracer("sqs").Start(ctx, "BatchDeleteProcessor.Process", trace.WithAttributes(
		attribute.Int("num_of_messages", len(msgs)),
	))
	defer span.End()

	msgCh := make(chan *types.Message)
	g, gctx := errgroup.WithContext(spanCtx)
	for _, msg := range msgs {
		msg := msg
		g.Go(func() error {
			err := p.inner.Process(gctx, msg)
			if err != nil {
				p.log.ErrorContext(gctx, "failed to process message", slogfield.Error(err))
				return nil
			}
			msgCh <- &msg
			return nil
		})
	}

	g2, _ := errgroup.WithContext(spanCtx)
	g2.Go(func() error {
		defer close(msgCh)
		return g.Wait()
	})

	deleteEntries := make([]types.DeleteMessageBatchRequestEntry, 0, len(msgs))
	g2.Go(func() error {
		for msg := range msgCh {
			if msg == nil {
				return nil
			}
			deleteEntries = append(deleteEntries, types.DeleteMessageBatchRequestEntry{
				ReceiptHandle: msg.ReceiptHandle,
				Id:            msg.MessageId,
			})
		}
		return nil
	})

	// Always delete try to delete messages even if
	// context has been cancelled.
	_ = g2.Wait()
	if len(deleteEntries) == 0 {
		return nil
	}

	resp, err := p.sqs.DeleteMessageBatch(spanCtx, &sqs.DeleteMessageBatchInput{
		QueueUrl: &p.queueUrl,
		Entries:  deleteEntries,
	})
	if err != nil {
		p.log.ErrorContext(
			spanCtx,
			"failed to batch delete messages",
			slogfield.Int("num_of_delete_entries", len(deleteEntries)),
			slogfield.Error(err),
		)
		return err
	}
	if len(resp.Failed) > 0 {
		for _, entry := range resp.Failed {
			p.log.ErrorContext(
				spanCtx,
				"failed to delete message",
				slogfield.String("sqs_message_id", *entry.Id),
				slogfield.String("sqs_error_code", *entry.Code),
				slogfield.String("sqs_error_message", *entry.Message),
				slogfield.Bool("sqs_sender_fault", entry.SenderFault),
			)
		}
	}
	return nil
}
