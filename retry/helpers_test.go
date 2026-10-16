// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package retry_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/z5labs/bedrock/retry"
)

var (
	errFail = errors.New("fail")
	errStop = errors.New("stop")
)

// classifyByError classifies nil as Success, errStop as Stop and every
// other error as Retry, with no least delay.
func classifyByError(_ context.Context, err error) (retry.Outcome, time.Duration) {
	switch {
	case err == nil:
		return retry.Success, 0
	case errors.Is(err, errStop):
		return retry.Stop, 0
	default:
		return retry.Retry, 0
	}
}

// fixed returns a schedule that always returns d.
func fixed(d time.Duration) func(int) time.Duration {
	return func(int) time.Duration { return d }
}

// scheduleRecorder is a schedule that records every attempt number it is
// called with, and returns delay(n).
type scheduleRecorder struct {
	delay func(int) time.Duration

	mu    sync.Mutex
	calls []int
}

func (s *scheduleRecorder) schedule(n int) time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, n)
	return s.delay(n)
}

func (s *scheduleRecorder) attempts() []int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]int(nil), s.calls...)
}

// classifierCounter wraps a classifier and counts its calls.
type classifierCounter struct {
	fn func(context.Context, error) (retry.Outcome, time.Duration)

	mu    sync.Mutex
	count int
}

func (c *classifierCounter) classify(ctx context.Context, err error) (retry.Outcome, time.Duration) {
	c.mu.Lock()
	c.count++
	c.mu.Unlock()
	return c.fn(ctx, err)
}

func (c *classifierCounter) calls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.count
}

func newRetrier(t *testing.T, maxAttempts int, schedule func(int) time.Duration, classify func(context.Context, error) (retry.Outcome, time.Duration), budget *retry.Budget) *retry.Retrier {
	t.Helper()

	r, err := retry.New(maxAttempts, schedule, classify, budget)
	require.NoError(t, err)
	require.NotNil(t, r)
	return r
}

func newBudget(t *testing.T, maxTokens int, ratio float64) *retry.Budget {
	t.Helper()

	b, err := retry.NewBudget(retry.BudgetConfig{MaxTokens: maxTokens, TokenRatio: ratio})
	require.NoError(t, err)
	return b
}

// call makes one call through r whose attempt n returns errs[n-1], or the
// last of errs once they run out, and returns the Decision of every attempt.
func call(r *retry.Retrier, ctx context.Context, errs ...error) []retry.Decision {
	var ds []retry.Decision
	for a := range r.Attempts(ctx) {
		err := errs[min(a.Number(), len(errs))-1]
		ds = append(ds, a.Done(err))
	}
	return ds
}

// catchPanic returns the value fn panicked with, or nil.
func catchPanic(fn func()) (v any) {
	defer func() { v = recover() }()
	fn()
	return nil
}
