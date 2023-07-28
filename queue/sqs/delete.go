// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package sqs

import (
	"context"

	"github.com/z5labs/app/queue"

	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

type batchDeleter interface {
	DeleteMessageBatch(context.Context, *sqs.DeleteMessageBatchInput, ...func(*sqs.Options)) (*sqs.DeleteMessageBatchOutput, error)
}

// BatchDeleteProcessor
type BatchDeleteProcessor struct {
	batchDeleter batchDeleter
}

// BatchDelete
func BatchDelete(url string, p queue.BatchProcessor[types.Message]) *BatchDeleteProcessor {
	return nil
}

// ProcessBatch
func (p *BatchDeleteProcessor) ProcessBatch(ctx context.Context, msgs []types.Message) []queue.BatchResult {
	return nil
}
