// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package retry_test

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/z5labs/bedrock/retry"
)

// drain makes n calls of one failing attempt through a Retrier of one
// attempt, each of which takes one token from b.
func drain(t *testing.T, b *retry.Budget, n int) {
	t.Helper()

	r := newRetrier(t, 1, fixed(0), classifyByError, b)
	for range n {
		require.Equal(t, []retry.Decision{retry.Exhausted}, call(r, t.Context(), errFail))
	}
}

func TestRetrier_budget(t *testing.T) {
	t.Parallel()

	t.Run("retries only while more than half of MaxTokens is left after the token is taken", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			b := newBudget(t, 4, 0.1)
			r := newRetrier(t, 10, fixed(time.Millisecond), classifyByError, b)

			// 4 tokens: the first failure leaves 3, more than 2, and the
			// second leaves 2, which is not.
			require.Equal(t, []retry.Decision{retry.Again, retry.Throttled}, call(r, t.Context(), errFail))
			require.Equal(t, 2.0, b.Tokens())
		})
	})

	t.Run("a throttled Budget still allows every first attempt", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			b := newBudget(t, 4, 0.1)
			r := newRetrier(t, 10, fixed(time.Millisecond), classifyByError, b)
			drain(t, b, 4)
			require.Zero(t, b.Tokens())

			for range 5 {
				require.Equal(t, []retry.Decision{retry.Throttled}, call(r, t.Context(), errFail))
			}
			require.Zero(t, b.Tokens())

			require.Equal(t, []retry.Decision{retry.Succeeded}, call(r, t.Context(), nil))
			require.Equal(t, 0.1, b.Tokens())
		})
	})

	t.Run("each Success adds TokenRatio exactly, up to MaxTokens", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			b := newBudget(t, 10, 0.1)
			r := newRetrier(t, 3, fixed(0), classifyByError, b)
			drain(t, b, 5)
			require.Equal(t, 5.0, b.Tokens())

			for range 10 {
				require.Equal(t, []retry.Decision{retry.Succeeded}, call(r, t.Context(), nil))
			}
			require.Equal(t, 6.0, b.Tokens())

			for range 100 {
				require.Equal(t, []retry.Decision{retry.Succeeded}, call(r, t.Context(), nil))
			}
			require.Equal(t, 10.0, b.Tokens())
		})
	})

	t.Run("TokenRatio is rounded to the nearest thousandth", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name  string
			ratio float64
			want  float64
		}{
			{name: "down", ratio: 0.1234, want: 5.123},
			{name: "up", ratio: 0.0016, want: 5.002},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				synctest.Test(t, func(t *testing.T) {
					b := newBudget(t, 10, tc.ratio)
					r := newRetrier(t, 3, fixed(0), classifyByError, b)
					drain(t, b, 5)

					require.Equal(t, []retry.Decision{retry.Succeeded}, call(r, t.Context(), nil))
					require.Equal(t, tc.want, b.Tokens())
				})
			})
		}
	})

	t.Run("Stop leaves the Budget as it was", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			b := newBudget(t, 10, 0.1)
			r := newRetrier(t, 3, fixed(0), classifyByError, b)
			drain(t, b, 1)

			require.Equal(t, []retry.Decision{retry.Stopped}, call(r, t.Context(), errStop))
			require.Equal(t, 9.0, b.Tokens())
		})
	})

	t.Run("an Exhausted Retry still takes a token", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			b := newBudget(t, 10, 0.1)
			r := newRetrier(t, 2, fixed(0), classifyByError, b)

			require.Equal(t, []retry.Decision{retry.Again, retry.Exhausted}, call(r, t.Context(), errFail))
			require.Equal(t, 8.0, b.Tokens())
		})
	})

	t.Run("a Retry Expired by a near deadline still takes a token", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			b := newBudget(t, 10, 0.1)
			r := newRetrier(t, 3, fixed(10*time.Millisecond), classifyByError, b)

			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Millisecond)
			defer cancel()

			require.Equal(t, []retry.Decision{retry.Expired}, call(r, ctx, errFail))
			require.Equal(t, 9.0, b.Tokens())
		})
	})

	t.Run("cancelled callers cost no token, and do not throttle another caller", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			b := newBudget(t, 4, 0.1)
			r := newRetrier(t, 3, fixed(time.Millisecond), classifyByError, b)

			cancelled, cancel := context.WithCancel(t.Context())
			cancel()
			for range 100 {
				require.Equal(t, []retry.Decision{retry.Expired}, call(r, cancelled, errFail))
			}
			require.Equal(t, 4.0, b.Tokens())

			require.Equal(t, []retry.Decision{retry.Again, retry.Succeeded}, call(r, t.Context(), errFail, nil))
		})
	})

	t.Run("one Budget shared by two Retriers", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			b := newBudget(t, 4, 0.1)
			r1 := newRetrier(t, 2, fixed(time.Millisecond), classifyByError, b)
			r2 := newRetrier(t, 3, fixed(time.Millisecond), classifyByError, b)

			require.Equal(t, []retry.Decision{retry.Again, retry.Exhausted}, call(r1, t.Context(), errFail))
			require.Equal(t, 2.0, b.Tokens())

			require.Equal(t, []retry.Decision{retry.Throttled}, call(r2, t.Context(), errFail))
			require.Equal(t, 1.0, b.Tokens())

			require.Equal(t, []retry.Decision{retry.Succeeded}, call(r2, t.Context(), nil))
			require.Equal(t, 1.1, b.Tokens())
		})
	})
}
