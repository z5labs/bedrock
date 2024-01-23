// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package sqs

import (
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

type sqsClient interface {
	sqsBatchDeleteClient
	sqsReceiveClient
}

type commonOptions struct {
	logHandler slog.Handler
	sqs        sqsClient
	queueUrl   string
}

// CommonOption are options common to all AWS SQS related
// consumers and processors.
type CommonOption interface {
	ConsumerOption
	BatchDeleteProcessorOption
}

type commonOptionFunc func(*commonOptions)

func (f commonOptionFunc) applyConsumer(co *consumerOptions) {
	f(&co.commonOptions)
}

func (f commonOptionFunc) applyProcessor(co *batchDeleteProcessorOptions) {
	f(&co.commonOptions)
}

// LogHandler configures the underlying slog.Handler.
func LogHandler(h slog.Handler) CommonOption {
	return commonOptionFunc(func(co *commonOptions) {
		co.logHandler = h
	})
}

// Client configures the underlying SQS client.
func Client(c *sqs.Client) CommonOption {
	return commonOptionFunc(func(co *commonOptions) {
		co.sqs = c
	})
}

// QueueUrl configures the SQS queue url.
func QueueUrl(url string) CommonOption {
	return commonOptionFunc(func(co *commonOptions) {
		co.queueUrl = url
	})
}
