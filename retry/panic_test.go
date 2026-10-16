// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package retry_test

import (
	"context"
	"runtime"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/z5labs/bedrock/retry"
)

func TestAttempt_misuse(t *testing.T) {
	t.Parallel()

	const withoutDone = "retry: attempt 1 ended without a call to Attempt.Done"

	t.Run("an attempt the body continues past without Done", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			r := newRetrier(t, 3, fixed(0), classifyByError, nil)

			require.PanicsWithValue(t, withoutDone, func() {
				for a := range r.Attempts(t.Context()) {
					_ = a.Number()
				}
			})
		})
	})

	t.Run("an attempt the body breaks out of without Done", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			r := newRetrier(t, 3, fixed(0), classifyByError, nil)

			require.PanicsWithValue(t, withoutDone, func() {
				for a := range r.Attempts(t.Context()) {
					_ = a.Number()
					break
				}
			})
		})
	})

	t.Run("an attempt the body returns from without Done", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			r := newRetrier(t, 3, fixed(0), classifyByError, nil)

			first := func() int {
				for a := range r.Attempts(t.Context()) {
					return a.Number()
				}
				return 0
			}
			require.PanicsWithValue(t, withoutDone, func() { _ = first() })
		})
	})

	t.Run("Done twice for one attempt", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			b := newBudget(t, 10, 0.1)
			r := newRetrier(t, 3, fixed(0), classifyByError, b)

			require.PanicsWithValue(t, "retry: Attempt.Done called twice for attempt 1", func() {
				for a := range r.Attempts(t.Context()) {
					a.Done(errFail)
					a.Done(errFail)
				}
			})
			require.Equal(t, 9.0, b.Tokens())
		})
	})

	t.Run("a second Done after the attempt ended with one", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			b := newBudget(t, 10, 0.1)
			r := newRetrier(t, 3, fixed(0), classifyByError, b)

			var last *retry.Attempt
			for a := range r.Attempts(t.Context()) {
				a.Done(errStop)
				last = a
			}

			require.PanicsWithValue(t, "retry: Attempt.Done called twice for attempt 1", func() {
				last.Done(errFail)
			})
			require.Equal(t, 10.0, b.Tokens())
		})
	})

	t.Run("Done after the attempt ended by a panic", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			b := newBudget(t, 10, 0.1)
			cc := &classifierCounter{fn: classifyByError}
			r := newRetrier(t, 3, fixed(0), cc.classify, b)

			var kept *retry.Attempt
			require.PanicsWithValue(t, "boom", func() {
				for a := range r.Attempts(t.Context()) {
					kept = a
					panic("boom")
				}
			})

			require.PanicsWithValue(t, "retry: Attempt.Done called after attempt 1 ended", func() {
				kept.Done(errFail)
			})
			require.Zero(t, cc.calls())
			require.Equal(t, 10.0, b.Tokens())
		})
	})

	t.Run("Done after the attempt ended by runtime.Goexit", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			b := newBudget(t, 10, 0.1)
			cc := &classifierCounter{fn: classifyByError}
			r := newRetrier(t, 3, fixed(0), cc.classify, b)

			var kept *retry.Attempt
			exited := make(chan struct{})
			go func() {
				defer close(exited)
				for a := range r.Attempts(t.Context()) {
					kept = a
					runtime.Goexit()
				}
			}()
			<-exited

			require.PanicsWithValue(t, "retry: Attempt.Done called after attempt 1 ended", func() {
				kept.Done(errFail)
			})
			require.Zero(t, cc.calls())
			require.Equal(t, 10.0, b.Tokens())
		})
	})

	t.Run("an attempt that ends before its Done returned", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			classify := func(context.Context, error) (retry.Outcome, time.Duration) {
				panic("classifier boom")
			}
			r := newRetrier(t, 3, fixed(0), classify, nil)

			var swallowed any
			require.PanicsWithValue(t, "retry: attempt 1 ended before its call to Attempt.Done returned", func() {
				for a := range r.Attempts(t.Context()) {
					swallowed = catchPanic(func() { a.Done(errFail) })
				}
			})
			require.Equal(t, "classifier boom", swallowed)
		})
	})
}

func TestRetrier_callerCodePanics(t *testing.T) {
	t.Parallel()

	t.Run("the classifier returns a value that is not an Outcome", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			outcome retry.Outcome
			want    string
		}{
			{outcome: retry.Outcome(0), want: "retry: the classifier returned Outcome(0), which is not an Outcome"},
			{outcome: retry.Outcome(4), want: "retry: the classifier returned Outcome(4), which is not an Outcome"},
		}

		for _, tc := range testCases {
			t.Run(tc.outcome.String(), func(t *testing.T) {
				t.Parallel()

				synctest.Test(t, func(t *testing.T) {
					b := newBudget(t, 10, 0.1)
					classify := func(context.Context, error) (retry.Outcome, time.Duration) {
						return tc.outcome, 0
					}
					r := newRetrier(t, 3, fixed(0), classify, b)

					require.PanicsWithValue(t, tc.want, func() {
						_ = call(r, t.Context(), errFail)
					})
					require.Equal(t, 10.0, b.Tokens())
				})
			})
		}
	})

	t.Run("the classifier returns a negative delay with Retry", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			b := newBudget(t, 10, 0.1)
			classify := func(context.Context, error) (retry.Outcome, time.Duration) {
				return retry.Retry, -time.Second
			}
			r := newRetrier(t, 3, fixed(0), classify, b)

			require.PanicsWithValue(t, "retry: the classifier returned a negative delay, -1s, for a Retry", func() {
				_ = call(r, t.Context(), errFail)
			})
			require.Equal(t, 10.0, b.Tokens())
		})
	})

	t.Run("the classifier panics, which leaves the Budget as it was", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			b := newBudget(t, 10, 0.1)
			classify := func(context.Context, error) (retry.Outcome, time.Duration) {
				panic("classifier boom")
			}
			r := newRetrier(t, 3, fixed(0), classify, b)

			require.PanicsWithValue(t, "classifier boom", func() {
				_ = call(r, t.Context(), errFail)
			})
			require.Equal(t, 10.0, b.Tokens())
		})
	})

	t.Run("the schedule returns a negative delay, after the token is taken", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			b := newBudget(t, 10, 0.1)
			r := newRetrier(t, 3, fixed(-time.Second), classifyByError, b)

			require.PanicsWithValue(t, "retry: the schedule returned a negative delay, -1s, for attempt 1", func() {
				_ = call(r, t.Context(), errFail)
			})
			require.Equal(t, 9.0, b.Tokens())
		})
	})
}

func TestRetrier_passThrough(t *testing.T) {
	t.Parallel()

	t.Run("a panic in the loop body passes through unchanged", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			type bodyPanic struct{ attempt int }
			want := &bodyPanic{attempt: 2}
			r := newRetrier(t, 3, fixed(time.Millisecond), classifyByError, nil)

			got := catchPanic(func() {
				for a := range r.Attempts(t.Context()) {
					if a.Number() == 2 {
						panic(want)
					}
					a.Done(errFail)
				}
			})
			require.Same(t, want, got)
		})
	})

	t.Run("runtime.Goexit in the loop body ends the call and the goroutine", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			r := newRetrier(t, 3, fixed(time.Millisecond), classifyByError, nil)

			var (
				attempts  int
				afterLoop bool
			)
			exited := make(chan struct{})
			go func() {
				defer close(exited)
				for a := range r.Attempts(t.Context()) {
					attempts++
					a.Done(errFail)
					runtime.Goexit()
				}
				afterLoop = true
			}()
			<-exited

			require.Equal(t, 1, attempts)
			require.False(t, afterLoop)
		})
	})
}
