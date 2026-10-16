// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package fit_test

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/z5labs/bedrock/fit"
)

func TestScenario_Match(t *testing.T) {
	t.Parallel()

	abort := fit.Fault{Abort: true}
	slow := fit.Fault{Delay: time.Second}

	testCases := []struct {
		name     string
		scenario fit.Scenario
		point    string
		want     fit.Rule
		wantOK   bool
	}{
		{
			name:     "exact match",
			scenario: fit.Scenario{Rules: []fit.Rule{{Point: "payments", Fault: abort}}},
			point:    "payments",
			want:     fit.Rule{Point: "payments", Fault: abort},
			wantOK:   true,
		},
		{
			name:     "exact rule does not match a longer name",
			scenario: fit.Scenario{Rules: []fit.Rule{{Point: "payments", Fault: abort}}},
			point:    "payments.refund",
		},
		{
			name:     "exact rule does not match a shorter name",
			scenario: fit.Scenario{Rules: []fit.Rule{{Point: "payments", Fault: abort}}},
			point:    "pay",
		},
		{
			name:     "prefix match with a trailing star",
			scenario: fit.Scenario{Rules: []fit.Rule{{Point: "payments.*", Fault: slow}}},
			point:    "payments.refund",
			want:     fit.Rule{Point: "payments.*", Fault: slow},
			wantOK:   true,
		},
		{
			name:     "prefix rule matches the bare prefix",
			scenario: fit.Scenario{Rules: []fit.Rule{{Point: "payments*", Fault: slow}}},
			point:    "payments",
			want:     fit.Rule{Point: "payments*", Fault: slow},
			wantOK:   true,
		},
		{
			name:     "prefix rule does not match another prefix",
			scenario: fit.Scenario{Rules: []fit.Rule{{Point: "payments.*", Fault: slow}}},
			point:    "orders.create",
		},
		{
			name:     "star inside a name is literal",
			scenario: fit.Scenario{Rules: []fit.Rule{{Point: "a*b", Fault: abort}}},
			point:    "axb",
		},
		{
			name:     "lone star matches every point",
			scenario: fit.Scenario{Rules: []fit.Rule{{Point: "*", Fault: abort}}},
			point:    "anything at all",
			want:     fit.Rule{Point: "*", Fault: abort},
			wantOK:   true,
		},
		{
			name:     "lone star matches the empty name",
			scenario: fit.Scenario{Rules: []fit.Rule{{Point: "*", Fault: abort}}},
			point:    "",
			want:     fit.Rule{Point: "*", Fault: abort},
			wantOK:   true,
		},
		{
			name: "first matching rule wins over a later one",
			scenario: fit.Scenario{Rules: []fit.Rule{
				{Point: "orders", Fault: slow},
				{Point: "payments.*", Fault: slow},
				{Point: "payments.refund", Fault: abort},
			}},
			point:  "payments.refund",
			want:   fit.Rule{Point: "payments.*", Fault: slow},
			wantOK: true,
		},
		{
			name: "zero Fault spares a point a later star would fail",
			scenario: fit.Scenario{Rules: []fit.Rule{
				{Point: "payments"},
				{Point: "*", Fault: abort},
			}},
			point:  "payments",
			want:   fit.Rule{Point: "payments"},
			wantOK: true,
		},
		{
			name: "later star fails a point the zero Fault does not spare",
			scenario: fit.Scenario{Rules: []fit.Rule{
				{Point: "payments"},
				{Point: "*", Fault: abort},
			}},
			point:  "orders",
			want:   fit.Rule{Point: "*", Fault: abort},
			wantOK: true,
		},
		{
			name: "no match",
			scenario: fit.Scenario{Rules: []fit.Rule{
				{Point: "payments", Fault: abort},
				{Point: "orders.*", Fault: slow},
			}},
			point: "inventory",
		},
		{
			name:     "empty Scenario matches nothing",
			scenario: fit.Scenario{},
			point:    "payments",
		},
		{
			name:     "empty Scenario matches nothing, not even the empty name",
			scenario: fit.Scenario{Rules: []fit.Rule{}},
			point:    "",
		},
		{
			name:     "name with spaces and braces matches exactly",
			scenario: fit.Scenario{Rules: []fit.Rule{{Point: "GET /users/{id}", Fault: abort}}},
			point:    "GET /users/{id}",
			want:     fit.Rule{Point: "GET /users/{id}", Fault: abort},
			wantOK:   true,
		},
		{
			name:     "name with brackets and colons matches exactly",
			scenario: fit.Scenario{Rules: []fit.Rule{{Point: "http:[::1]:8080", Fault: abort}}},
			point:    "http:[::1]:8080",
			want:     fit.Rule{Point: "http:[::1]:8080", Fault: abort},
			wantOK:   true,
		},
		{
			name:     "name with a greater-than sign matches exactly",
			scenario: fit.Scenario{Rules: []fit.Rule{{Point: "nats:orders.>", Fault: abort}}},
			point:    "nats:orders.>",
			want:     fit.Rule{Point: "nats:orders.>", Fault: abort},
			wantOK:   true,
		},
		{
			name:     "name the old character set rejected does not match a near miss",
			scenario: fit.Scenario{Rules: []fit.Rule{{Point: "GET /users/{id}", Fault: abort}}},
			point:    "GET /users/42",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, ok := tc.scenario.Match(tc.point)
			require.Equal(t, tc.wantOK, ok)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestFault_Wait(t *testing.T) {
	t.Parallel()

	t.Run("zero or negative Delay returns at once", func(t *testing.T) {
		t.Parallel()

		for _, delay := range []time.Duration{0, -time.Nanosecond, -time.Hour} {
			synctest.Test(t, func(t *testing.T) {
				start := time.Now()
				err := fit.Fault{Delay: delay}.Wait(t.Context())
				require.NoError(t, err)
				require.Zero(t, time.Since(start))
			})
		}
	})

	t.Run("zero Delay returns nil even on a done context", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			ctx, cancel := context.WithCancelCause(t.Context())
			cancel(errors.New("stopped"))

			require.NoError(t, fit.Fault{}.Wait(ctx))
		})
	})

	t.Run("positive Delay returns after exactly that much time", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			start := time.Now()
			err := fit.Fault{Delay: 3 * time.Second}.Wait(t.Context())
			require.NoError(t, err)
			require.Equal(t, 3*time.Second, time.Since(start))
		})
	})

	t.Run("cancelled context returns its cause before the delay ends", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			cause := errors.New("client went away")
			ctx, cancel := context.WithCancelCause(t.Context())
			time.AfterFunc(time.Second, func() { cancel(cause) })

			start := time.Now()
			err := fit.Fault{Delay: time.Minute}.Wait(ctx)
			require.ErrorIs(t, err, cause)
			require.Equal(t, time.Second, time.Since(start))
		})
	})
}

func TestFault_Inject(t *testing.T) {
	t.Parallel()

	t.Run("zero Fault returns nil at once", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			start := time.Now()
			require.NoError(t, fit.Fault{}.Inject(t.Context()))
			require.Zero(t, time.Since(start))
		})
	})

	t.Run("Abort with zero or negative Delay returns ErrInjected at once", func(t *testing.T) {
		t.Parallel()

		for _, delay := range []time.Duration{0, -time.Second} {
			synctest.Test(t, func(t *testing.T) {
				start := time.Now()
				err := fit.Fault{Delay: delay, Abort: true}.Inject(t.Context())
				require.ErrorIs(t, err, fit.ErrInjected)
				require.Zero(t, time.Since(start))
			})
		}
	})

	t.Run("Delay without Abort returns nil after the delay", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			start := time.Now()
			require.NoError(t, fit.Fault{Delay: 2 * time.Second}.Inject(t.Context()))
			require.Equal(t, 2*time.Second, time.Since(start))
		})
	})

	t.Run("Abort returns ErrInjected only after the delay", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			done := make(chan error, 1)
			go func() { done <- fit.Fault{Delay: 5 * time.Second, Abort: true}.Inject(t.Context()) }()

			time.Sleep(5*time.Second - time.Nanosecond)
			synctest.Wait()
			select {
			case err := <-done:
				require.FailNow(t, "Inject returned before the delay ended", "returned %v", err)
			default:
			}

			time.Sleep(time.Nanosecond)
			synctest.Wait()
			select {
			case err := <-done:
				require.ErrorIs(t, err, fit.ErrInjected)
			default:
				require.FailNow(t, "Inject did not return when the delay ended")
			}
		})
	})

	t.Run("cancelled context returns its cause before the delay ends", func(t *testing.T) {
		t.Parallel()

		for _, abort := range []bool{false, true} {
			synctest.Test(t, func(t *testing.T) {
				cause := errors.New("deadline from upstream")
				ctx, cancel := context.WithCancelCause(t.Context())
				time.AfterFunc(time.Second, func() { cancel(cause) })

				start := time.Now()
				err := fit.Fault{Delay: time.Minute, Abort: abort}.Inject(ctx)
				require.ErrorIs(t, err, cause)
				require.NotErrorIs(t, err, fit.ErrInjected)
				require.Equal(t, time.Second, time.Since(start))
			})
		}
	})
}
