// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package retry_test

import (
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/z5labs/bedrock/retry"
)

func TestExponential_rejects_invalid_arguments(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		base     time.Duration
		maxDelay time.Duration
		want     string
	}{
		{
			name:     "zero base",
			base:     0,
			maxDelay: time.Second,
			want:     "retry: base delay must be positive, got 0s",
		},
		{
			name:     "max delay below the base",
			base:     100 * time.Millisecond,
			maxDelay: 50 * time.Millisecond,
			want:     "retry: max delay must be at least the base delay, 100ms, got 50ms",
		},
		{
			name:     "both invalid",
			base:     0,
			maxDelay: -time.Second,
			want: "retry: base delay must be positive, got 0s\n" +
				"retry: max delay must be at least the base delay, 0s, got -1s",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			schedule, err := retry.Exponential(tc.base, tc.maxDelay)
			require.Nil(t, schedule)
			require.EqualError(t, err, tc.want)
		})
	}
}

func TestExponential_draws_within_the_backoff(t *testing.T) {
	t.Parallel()

	const (
		base     = 100 * time.Millisecond
		maxDelay = time.Second
		draws    = 1000
	)

	schedule, err := retry.Exponential(base, maxDelay)
	require.NoError(t, err)

	testCases := []struct {
		attempt int
		ceiling time.Duration
	}{
		{attempt: 1, ceiling: 100 * time.Millisecond},
		{attempt: 2, ceiling: 200 * time.Millisecond},
		{attempt: 3, ceiling: 400 * time.Millisecond},
		{attempt: 4, ceiling: 800 * time.Millisecond},
		{attempt: 5, ceiling: time.Second},
		{attempt: 63, ceiling: time.Second},
		{attempt: 64, ceiling: time.Second},
		{attempt: math.MaxInt, ceiling: time.Second},
	}

	for _, tc := range testCases {
		t.Run(strconv.Itoa(tc.attempt), func(t *testing.T) {
			t.Parallel()

			upperHalf := false
			for range draws {
				d := schedule(tc.attempt)
				require.GreaterOrEqual(t, d, time.Duration(0))
				require.Less(t, d, tc.ceiling)
				if d >= tc.ceiling/2 {
					upperHalf = true
				}
			}
			require.True(t, upperHalf, "no draw for attempt %d fell in [%v, %v)", tc.attempt, tc.ceiling/2, tc.ceiling)
		})
	}
}

func TestExponential_never_overflows(t *testing.T) {
	t.Parallel()

	const largest = time.Duration(math.MaxInt64)

	schedule, err := retry.Exponential(largest, largest)
	require.NoError(t, err)

	for _, attempt := range []int{1, 2, 63, 64, 65, math.MaxInt} {
		d := schedule(attempt)
		require.GreaterOrEqual(t, d, time.Duration(0), "attempt %d", attempt)
		require.Less(t, d, largest, "attempt %d", attempt)
	}
}

func TestExponential_panics_on_an_attempt_below_1(t *testing.T) {
	t.Parallel()

	schedule, err := retry.Exponential(100*time.Millisecond, time.Second)
	require.NoError(t, err)

	testCases := []struct {
		attempt int
		want    string
	}{
		{attempt: 0, want: "retry: schedule called with attempt 0, which is less than 1"},
		{attempt: -1, want: "retry: schedule called with attempt -1, which is less than 1"},
	}

	for _, tc := range testCases {
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()

			require.PanicsWithValue(t, tc.want, func() {
				_ = schedule(tc.attempt)
			})
		})
	}
}
