// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package sqs

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

// QueueConfig
type QueueConfig struct {
	URL                 string `config:"queue_url"`
	MaxNumberOfMessages int32  `config:"max_number_of_messages"`
	WaitTimeSeconds     int32  `config:"wait_time_seconds"`
	VisibilityTimeout   int32  `config:"visibility_timeout"`
}

type receiver interface {
	ReceiveMessage(context.Context, *sqs.ReceiveMessageInput, ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error)
}

// Queue
type Queue struct {
	receiver            receiver
	queueURL            string
	maxNumberOfMessages int32
	waitTimeSeconds     int32
	visibilityTimeout   int32
}

// Dial
func Dial(url string) *Queue {
	return nil
}
