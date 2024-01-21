// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package pubsub

import (
	"log/slog"

	pubsub "cloud.google.com/go/pubsub/apiv1"
)

type pubsubClient interface {
	pubsubPullClient
	pubsubAckClient
}

type commonOptions struct {
	logHandler   slog.Handler
	pubsub       pubsubClient
	subscription string
}

// CommonOption are options common to all Google Cloud PubSub related
// consumers and processors.
type CommonOption interface {
	ConsumerOption
	BatchAcknowledgeProcessorOption
}

type commonOptionFunc func(*commonOptions)

func (f commonOptionFunc) applyConsumer(co *consumerOptions) {
	f(&co.commonOptions)
}

func (f commonOptionFunc) applyProcessor(bo *batchAcknowledgeProcessorOptions) {
	f(&bo.commonOptions)
}

// LogHandler configures the underlying slog.Handler.
func LogHandler(h slog.Handler) CommonOption {
	return commonOptionFunc(func(co *commonOptions) {
		co.logHandler = h
	})
}

// Client configures the underlying PubSub client.
func Client(c *pubsub.SubscriberClient) CommonOption {
	return commonOptionFunc(func(co *commonOptions) {
		co.pubsub = c
	})
}

// Subscription configures the PubSub subscription id.
func Subscription(s string) CommonOption {
	return commonOptionFunc(func(co *commonOptions) {
		co.subscription = s
	})
}
