// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package retry_test

import (
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/z5labs/bedrock/retry"
)

func TestRetrier_concurrency(t *testing.T) {
	t.Parallel()

	const goroutines = 16

	t.Run("failing calls on one Budget take exactly one token per Retry", func(t *testing.T) {
		t.Parallel()

		const calls = 200

		b := newBudget(t, retry.TokenLimit, 0.001)
		r := newRetrier(t, 3, fixed(0), classifyByError, b)

		var violations atomic.Int32
		var wg sync.WaitGroup
		for range goroutines {
			wg.Go(func() {
				for range calls {
					ds := call(r, t.Context(), errFail)
					if len(ds) != 3 || ds[0] != retry.Again || ds[1] != retry.Again || ds[2] != retry.Exhausted {
						violations.Add(1)
					}
				}
			})
		}
		wg.Wait()

		require.Zero(t, violations.Load())
		require.Equal(t, float64(retry.TokenLimit-goroutines*calls*3), b.Tokens())
		require.Equal(t, 9600, goroutines*calls*3)
	})

	t.Run("mixed results keep the Budget in range", func(t *testing.T) {
		t.Parallel()

		const (
			calls     = 500
			maxTokens = 10
		)

		b := newBudget(t, maxTokens, 0.5)
		r := newRetrier(t, 4, fixed(0), classifyByError, b)
		results := []error{nil, errFail, errFail, errStop}

		var violations atomic.Int32
		var wg sync.WaitGroup
		for range goroutines {
			wg.Go(func() {
				for range calls {
					for a := range r.Attempts(t.Context()) {
						if a.Done(results[rand.IntN(len(results))]) == 0 {
							violations.Add(1)
						}
					}
					if tokens := b.Tokens(); tokens < 0 || tokens > maxTokens {
						violations.Add(1)
					}
				}
			})
		}
		wg.Wait()

		require.Zero(t, violations.Load())
	})

	t.Run("Done on another goroutine racing the end of its attempt", func(t *testing.T) {
		t.Parallel()

		const rounds = 20_000

		// Large enough that no round empties it, so every Done that returned
		// shows up as exactly one token.
		b := newBudget(t, retry.TokenLimit, 0.001)
		r := newRetrier(t, 1, fixed(0), classifyByError, b)

		allowed := map[any]bool{
			"retry: attempt 1 ended without a call to Attempt.Done":           true,
			"retry: attempt 1 ended before its call to Attempt.Done returned": true,
			"retry: Attempt.Done called after attempt 1 ended":                true,
		}

		var (
			returned   int
			unexpected []any
		)
		for range rounds {
			var (
				wg      sync.WaitGroup
				doneRet bool
				donePan any
			)
			endPan := catchPanic(func() {
				for a := range r.Attempts(t.Context()) {
					wg.Go(func() {
						donePan = catchPanic(func() {
							a.Done(errFail)
							doneRet = true
						})
					})
					break
				}
			})
			wg.Wait()

			if doneRet {
				returned++
			}
			for _, p := range []any{endPan, donePan} {
				if p != nil && !allowed[p] {
					unexpected = append(unexpected, p)
				}
			}
		}

		require.Empty(t, unexpected)
		require.Equal(t, float64(retry.TokenLimit-returned), b.Tokens())
	})
}
