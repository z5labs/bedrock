// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package queue

import "context"

// BatchResult
type BatchResult struct {
	Err error
}

// BatchProcessor
type BatchProcessor[T any] interface {
	ProcessBatch(context.Context, []T) []BatchResult
}
