// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package sqs

import (
	"context"
	"log/slog"

	"github.com/z5labs/bedrock/pkg/noop"
	"github.com/z5labs/bedrock/pkg/otelslog"
	"github.com/z5labs/bedrock/pkg/slogfield"
	"github.com/z5labs/bedrock/queue"
	"github.com/z5labs/bedrock/queue/sqs/sqsslog"

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

// ConsumerOption are options for configuring the Consumer.
type ConsumerOption interface {
	applyConsumer(*consumerOptions)
}

type consumerOptionFunc func(*consumerOptions)

func (f consumerOptionFunc) applyConsumer(co *consumerOptions) {
	f(co)
}

// MaxNumOfMessages defines the maximum number of messages which
// Amazon SQS will return in a single response.
//
// Amazon SQS never returns more messages than this value (however,
// fewer messages might be returned). The minimum is 1. The maximum is 10.
func MaxNumOfMessages(n int32) ConsumerOption {
	return consumerOptionFunc(func(co *consumerOptions) {
		co.maxNumOfMessages = n
	})
}

// VisibilityTimeout a period of time during which Amazon SQS prevents
// all consumers from receiving and processing the message.
//
// The default visibility timeout for a message is 30 seconds.
// The minimum is 0 seconds. The maximum is 12 hours.
func VisibilityTimeout(n int32) ConsumerOption {
	return consumerOptionFunc(func(co *consumerOptions) {
		co.visibilityTimeout = n
	})
}

// WaitTimeSeconds is the duration (in seconds) for which the call
// waits for a message to arrive in the queue before returning. If
// a message is available, the call returns sooner than WaitTimeSeconds.
// If no messages are available and the wait time expires, the call
// returns successfully with an empty list of messages. To avoid HTTP
// errors, ensure that the HTTP response timeout for ReceiveMessage requests is
// longer than the WaitTimeSeconds parameter.
func WaitTimeSeconds(n int32) ConsumerOption {
	return consumerOptionFunc(func(co *consumerOptions) {
		co.waitTimeSeconds = n
	})
}

type sqsReceiveClient interface {
	ReceiveMessage(context.Context, *sqs.ReceiveMessageInput, ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error)
}

// Consumer consumes messages from AWS SQS.
type Consumer struct {
	log *slog.Logger
	sqs sqsReceiveClient

	queueUrl          string
	maxNumOfMessages  int32
	visibilityTimeout int32
	waitTimeSeconds   int32
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
		log:               otelslog.New(co.logHandler),
		sqs:               co.sqs,
		queueUrl:          co.queueUrl,
		maxNumOfMessages:  co.maxNumOfMessages,
		visibilityTimeout: co.visibilityTimeout,
		waitTimeSeconds:   co.waitTimeSeconds,
	}
}

// Consume implements the queue.Consumer interface.
//
// A ReceiveMessage request is sent to AWS SQS with the
// configured options (e.g. visibility timeout, wait time seconds, etc.).
// An error is only returned in the case where the SQS request
// fails or SQS returns zero messages. In the case of the zero messages,
// the error, queue.ErrNoItem, is returned which allows the queue based
// runtimes to disregard this as a failure and retry consuming messages.
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

// BatchDeleteProcessorOption are options for configuring the BatchDeleteProcessor.
type BatchDeleteProcessorOption interface {
	applyProcessor(*batchDeleteProcessorOptions)
}

type batchDeleteProcessorOptionFunc func(*batchDeleteProcessorOptions)

func (f batchDeleteProcessorOptionFunc) applyProcessor(bo *batchDeleteProcessorOptions) {
	f(bo)
}

// Processor configures the underlying single message queue.Processor
// which the BatchDeleteProcessor calls when concurrently processing a
// batch of messages.
func Processor(p queue.Processor[types.Message]) BatchDeleteProcessorOption {
	return batchDeleteProcessorOptionFunc(func(bo *batchDeleteProcessorOptions) {
		bo.inner = p
	})
}

type sqsBatchDeleteClient interface {
	DeleteMessageBatch(context.Context, *sqs.DeleteMessageBatchInput, ...func(*sqs.Options)) (*sqs.DeleteMessageBatchOutput, error)
}

// BatchDeleteProcessor will concurrently process and delete messages from AWS SQS.
type BatchDeleteProcessor struct {
	log *slog.Logger
	sqs sqsBatchDeleteClient

	queueUrl string
	inner    queue.Processor[types.Message]
}

// NewBatchDeleteProcessor returns a fully initially BatchDeleteProcessor.
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

// Process implements the queue.Processor interface.
//
// Each message is processed concurrently using the processor
// that was provided to the BatchDeleteProcessor when it was
// created. If the inner processor returns an error for a message,
// it will not be deleted from SQS and will be reprocessed after
// the VisibilityTimeout expires. If no error is returned, the
// message will be collected with the other messages from the slice,
// msgs, to be deleted together in a single BatchDelete request to SQS.
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

	// Always try delete to delete messages even if
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
				sqsslog.MessageId(deref(entry.Id)),
				slogfield.String("sqs_error_code", *entry.Code),
				slogfield.String("sqs_error_message", *entry.Message),
				slogfield.Bool("sqs_sender_fault", entry.SenderFault),
			)
		}
	}
	return nil
}

func deref[T any](t *T) T {
	var zero T
	if t == nil {
		return zero
	}
	return *t
}
