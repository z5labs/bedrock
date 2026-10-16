// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package retry_test

import (
	"context"
	"math"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/z5labs/bedrock/retry"
)

func TestNew(t *testing.T) {
	t.Parallel()

	t.Run("panics on an invalid argument before checking maxAttempts", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name     string
			schedule func(int) time.Duration
			classify func(context.Context, error) (retry.Outcome, time.Duration)
			budget   *retry.Budget
			want     string
		}{
			{
				name:     "nil schedule",
				schedule: nil,
				classify: classifyByError,
				want:     "retry: New called with a nil schedule",
			},
			{
				name:     "nil classify function",
				schedule: fixed(0),
				classify: nil,
				want:     "retry: New called with a nil classify function",
			},
			{
				name:     "zero Budget",
				schedule: fixed(0),
				classify: classifyByError,
				budget:   &retry.Budget{},
				want:     "retry: New called with a Budget not built by NewBudget",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				require.PanicsWithValue(t, tc.want, func() {
					_, _ = retry.New(0, tc.schedule, tc.classify, tc.budget)
				})
			})
		}
	})

	t.Run("returns an error for maxAttempts below 1", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			maxAttempts int
			want        string
		}{
			{maxAttempts: 0, want: "retry: max attempts must be at least 1, got 0"},
			{maxAttempts: -1, want: "retry: max attempts must be at least 1, got -1"},
		}

		for _, tc := range testCases {
			t.Run(tc.want, func(t *testing.T) {
				t.Parallel()

				r, err := retry.New(tc.maxAttempts, fixed(0), classifyByError, newBudget(t, 10, 0.1))
				require.Nil(t, r)
				require.EqualError(t, err, tc.want)
			})
		}
	})

	t.Run("accepts 1 and math.MaxInt, with and without a Budget", func(t *testing.T) {
		t.Parallel()

		for _, maxAttempts := range []int{1, math.MaxInt} {
			for _, withBudget := range []bool{false, true} {
				var budget *retry.Budget
				if withBudget {
					budget = newBudget(t, 10, 0.1)
				}

				r, err := retry.New(maxAttempts, fixed(0), classifyByError, budget)
				require.NoError(t, err)
				require.NotNil(t, r)
				require.Equal(t, []retry.Decision{retry.Succeeded}, call(r, t.Context(), nil))
			}
		}
	})
}

func TestRetrier_argumentAssertions(t *testing.T) {
	t.Parallel()

	t.Run("Attempts with a nil context panics", func(t *testing.T) {
		t.Parallel()

		r := newRetrier(t, 3, fixed(0), classifyByError, nil)

		var nilCtx context.Context
		require.PanicsWithValue(t, "retry: Retrier.Attempts called with a nil context", func() {
			_ = r.Attempts(nilCtx)
		})
	})

	retriers := []struct {
		name string
		r    *retry.Retrier
	}{
		{name: "nil Retrier", r: nil},
		{name: "zero Retrier", r: &retry.Retrier{}},
	}

	for _, tc := range retriers {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.PanicsWithValue(t, "retry: Retrier.Attempts called on a Retrier not built by New", func() {
				_ = tc.r.Attempts(t.Context())
			})
		})
	}

	attempts := []struct {
		name string
		a    *retry.Attempt
	}{
		{name: "nil Attempt", a: nil},
		{name: "zero Attempt", a: &retry.Attempt{}},
	}

	for _, tc := range attempts {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.PanicsWithValue(t, "retry: Attempt.Number called on an Attempt not yielded by Retrier.Attempts", func() {
				_ = tc.a.Number()
			})
			require.PanicsWithValue(t, "retry: Attempt.Done called on an Attempt not yielded by Retrier.Attempts", func() {
				_ = tc.a.Done(nil)
			})
		})
	}
}

func TestRetrier_loop(t *testing.T) {
	t.Parallel()

	t.Run("a first-attempt success ends the call", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			sched := &scheduleRecorder{delay: fixed(time.Second)}
			r := newRetrier(t, 3, sched.schedule, classifyByError, nil)

			start := time.Now()
			require.Equal(t, []retry.Decision{retry.Succeeded}, call(r, t.Context(), nil))
			require.Zero(t, time.Since(start))
			require.Empty(t, sched.attempts())
		})
	})

	t.Run("retries until a success, waiting as the schedule says", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			sched := &scheduleRecorder{delay: func(n int) time.Duration { return time.Duration(n) * 10 * time.Millisecond }}
			r := newRetrier(t, 5, sched.schedule, classifyByError, nil)

			start := time.Now()
			var (
				ds      []retry.Decision
				numbers []int
				starts  []time.Duration
			)
			for a := range r.Attempts(t.Context()) {
				numbers = append(numbers, a.Number())
				starts = append(starts, time.Since(start))
				err := errFail
				if a.Number() == 3 {
					err = nil
				}
				ds = append(ds, a.Done(err))
			}

			require.Equal(t, []retry.Decision{retry.Again, retry.Again, retry.Succeeded}, ds)
			require.Equal(t, []int{1, 2, 3}, numbers)
			require.Equal(t, []time.Duration{0, 10 * time.Millisecond, 30 * time.Millisecond}, starts)
			require.Equal(t, []int{1, 2}, sched.attempts())
			require.Equal(t, 30*time.Millisecond, time.Since(start))
		})
	})

	t.Run("Stop ends the call", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			sched := &scheduleRecorder{delay: fixed(time.Second)}
			r := newRetrier(t, 3, sched.schedule, classifyByError, nil)

			start := time.Now()
			require.Equal(t, []retry.Decision{retry.Again, retry.Stopped}, call(r, t.Context(), errFail, errStop))
			require.Equal(t, time.Second, time.Since(start))
			require.Equal(t, []int{1}, sched.attempts())
		})
	})

	t.Run("a Retry on the last allowed attempt is Exhausted without calling the schedule", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			sched := &scheduleRecorder{delay: fixed(time.Millisecond)}
			r := newRetrier(t, 3, sched.schedule, classifyByError, nil)

			require.Equal(t, []retry.Decision{retry.Again, retry.Again, retry.Exhausted}, call(r, t.Context(), errFail))
			require.Equal(t, []int{1, 2}, sched.attempts())
		})
	})

	t.Run("a maxAttempts of 1 never retries", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			sched := &scheduleRecorder{delay: fixed(time.Millisecond)}
			r := newRetrier(t, 1, sched.schedule, classifyByError, nil)

			start := time.Now()
			require.Equal(t, []retry.Decision{retry.Exhausted}, call(r, t.Context(), errFail))
			require.Zero(t, time.Since(start))
			require.Empty(t, sched.attempts())
		})
	})

	t.Run("the delay is the larger of the schedule's and the classifier's", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name  string
			after time.Duration
			want  time.Duration
		}{
			{name: "the classifier's is longer", after: 50 * time.Millisecond, want: 50 * time.Millisecond},
			{name: "the schedule's is longer", after: 5 * time.Millisecond, want: 10 * time.Millisecond},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				synctest.Test(t, func(t *testing.T) {
					classify := func(_ context.Context, err error) (retry.Outcome, time.Duration) {
						if err == nil {
							return retry.Success, 0
						}
						return retry.Retry, tc.after
					}
					r := newRetrier(t, 3, fixed(10*time.Millisecond), classify, nil)

					start := time.Now()
					require.Equal(t, []retry.Decision{retry.Again, retry.Succeeded}, call(r, t.Context(), errFail, nil))
					require.Equal(t, tc.want, time.Since(start))
				})
			})
		}
	})

	t.Run("the classifier's delay is ignored for Success and Stop, even when negative", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			outcome retry.Outcome
			want    retry.Decision
		}{
			{outcome: retry.Success, want: retry.Succeeded},
			{outcome: retry.Stop, want: retry.Stopped},
		}

		for _, tc := range testCases {
			for _, after := range []time.Duration{time.Hour, -time.Hour} {
				t.Run(tc.outcome.String()+" "+after.String(), func(t *testing.T) {
					t.Parallel()

					synctest.Test(t, func(t *testing.T) {
						classify := func(context.Context, error) (retry.Outcome, time.Duration) {
							return tc.outcome, after
						}
						r := newRetrier(t, 3, fixed(time.Millisecond), classify, newBudget(t, 10, 0.1))

						start := time.Now()
						require.Equal(t, []retry.Decision{tc.want}, call(r, t.Context(), errFail))
						require.Zero(t, time.Since(start))
					})
				})
			}
		}
	})

	t.Run("a zero delay retries at once", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			r := newRetrier(t, 3, fixed(0), classifyByError, nil)

			start := time.Now()
			require.Equal(t, []retry.Decision{retry.Again, retry.Again, retry.Exhausted}, call(r, t.Context(), errFail))
			require.Zero(t, time.Since(start))
		})
	})

	t.Run("each range over the sequence is a new call from attempt 1", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			r := newRetrier(t, 2, fixed(time.Millisecond), classifyByError, nil)
			seq := r.Attempts(t.Context())

			for range 2 {
				var numbers []int
				var ds []retry.Decision
				for a := range seq {
					numbers = append(numbers, a.Number())
					ds = append(ds, a.Done(errFail))
				}
				require.Equal(t, []int{1, 2}, numbers)
				require.Equal(t, []retry.Decision{retry.Again, retry.Exhausted}, ds)
			}
		})
	})

	t.Run("the classifier receives the context passed to Attempts", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			type key struct{}
			ctx := context.WithValue(t.Context(), key{}, "the call's")

			var got []any
			classify := func(ctx context.Context, err error) (retry.Outcome, time.Duration) {
				got = append(got, ctx.Value(key{}))
				return classifyByError(ctx, err)
			}
			r := newRetrier(t, 3, fixed(time.Millisecond), classify, nil)

			require.Equal(t, []retry.Decision{retry.Again, retry.Succeeded}, call(r, ctx, errFail, nil))
			require.Equal(t, []any{"the call's", "the call's"}, got)
		})
	})
}

func TestRetrier_context(t *testing.T) {
	t.Parallel()

	t.Run("a Retry with the context done is Expired before the attempt limit or the Budget is checked", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			// A Budget one failure from throttling, and a Retrier of one
			// attempt, either of which would decide otherwise.
			b := newBudget(t, 4, 0.1)
			sched := &scheduleRecorder{delay: fixed(time.Millisecond)}
			r := newRetrier(t, 1, sched.schedule, classifyByError, b)

			ctx, cancel := context.WithCancel(t.Context())
			cancel()

			require.Equal(t, []retry.Decision{retry.Expired}, call(r, ctx, errFail))
			require.Equal(t, 4.0, b.Tokens())
			require.Empty(t, sched.attempts())
		})
	})

	t.Run("a context already done still gets its first attempt", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			r := newRetrier(t, 3, fixed(time.Millisecond), classifyByError, nil)

			ctx, cancel := context.WithCancel(t.Context())
			cancel()

			require.Equal(t, []retry.Decision{retry.Succeeded}, call(r, ctx, nil))
		})
	})

	t.Run("a deadline at or before the next attempt is Expired", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name    string
			timeout time.Duration
			want    []retry.Decision
		}{
			{name: "before", timeout: 9 * time.Millisecond, want: []retry.Decision{retry.Expired}},
			{name: "exactly at", timeout: 10 * time.Millisecond, want: []retry.Decision{retry.Expired}},
			{name: "after", timeout: 11 * time.Millisecond, want: []retry.Decision{retry.Again, retry.Succeeded}},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				synctest.Test(t, func(t *testing.T) {
					r := newRetrier(t, 3, fixed(10*time.Millisecond), classifyByError, nil)

					ctx, cancel := context.WithTimeout(t.Context(), tc.timeout)
					defer cancel()

					require.Equal(t, tc.want, call(r, ctx, errFail, nil))
				})
			})
		}
	})

	t.Run("the classifier's delay counts against the deadline", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			classify := func(context.Context, error) (retry.Outcome, time.Duration) {
				return retry.Retry, 20 * time.Millisecond
			}
			r := newRetrier(t, 3, fixed(time.Millisecond), classify, nil)

			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
			defer cancel()

			start := time.Now()
			require.Equal(t, []retry.Decision{retry.Expired}, call(r, ctx, errFail))
			require.Zero(t, time.Since(start))
		})
	})

	t.Run("a cancellation during the wait ends the call at once", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			r := newRetrier(t, 3, fixed(100*time.Millisecond), classifyByError, nil)

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			time.AfterFunc(30*time.Millisecond, cancel)

			start := time.Now()
			require.Equal(t, []retry.Decision{retry.Again}, call(r, ctx, errFail))
			require.Equal(t, 30*time.Millisecond, time.Since(start))
			require.ErrorIs(t, ctx.Err(), context.Canceled)
		})
	})

	t.Run("a cancellation after an Again with a zero delay ends the call", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			r := newRetrier(t, 3, fixed(0), classifyByError, nil)

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()

			var ds []retry.Decision
			for a := range r.Attempts(ctx) {
				ds = append(ds, a.Done(errFail))
				cancel()
			}

			require.Equal(t, []retry.Decision{retry.Again}, ds)
			require.ErrorIs(t, ctx.Err(), context.Canceled)
		})
	})
}

func TestRetrier_loopBody(t *testing.T) {
	t.Parallel()

	t.Run("a break after Again skips the wait", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			r := newRetrier(t, 3, fixed(time.Hour), classifyByError, nil)

			start := time.Now()
			var ds []retry.Decision
			for a := range r.Attempts(t.Context()) {
				ds = append(ds, a.Done(errFail))
				break
			}

			require.Equal(t, []retry.Decision{retry.Again}, ds)
			require.Zero(t, time.Since(start))
		})
	})

	t.Run("a return from the enclosing function ends the call", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			r := newRetrier(t, 3, fixed(time.Hour), classifyByError, nil)

			attempts := 0
			first := func() retry.Decision {
				for a := range r.Attempts(t.Context()) {
					attempts++
					return a.Done(errFail)
				}
				return 0
			}

			start := time.Now()
			require.Equal(t, retry.Again, first())
			require.Equal(t, 1, attempts)
			require.Zero(t, time.Since(start))
		})
	})

	t.Run("Done on another goroutine before the attempt ends", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			r := newRetrier(t, 3, fixed(time.Millisecond), classifyByError, nil)

			var ds []retry.Decision
			for a := range r.Attempts(t.Context()) {
				err := errFail
				if a.Number() == 2 {
					err = nil
				}

				done := make(chan retry.Decision)
				go func() { done <- a.Done(err) }()
				ds = append(ds, <-done)
			}

			require.Equal(t, []retry.Decision{retry.Again, retry.Succeeded}, ds)
		})
	})
}
