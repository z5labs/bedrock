// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package circuit_test

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/z5labs/bedrock/circuit"
)

// reopenOnTimeout reopens when a probe times out, and otherwise behaves as
// closeOnSuccess.
func reopenOnTimeout(c circuit.Counts) circuit.State {
	if c.TimedOut > 0 {
		return circuit.Open
	}
	return closeOnSuccess(c)
}

func TestBreaker_probeTimeout(t *testing.T) {
	t.Parallel()

	t.Run("a never-settled probe holds its slot until exactly ProbeTimeout", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			pol := &policyRecorder{trip: tripOn(1), probe: stayHalfOpen}
			b := newHalfOpen(t, pol, classifyByError)
			ctx := t.Context()

			mustAllow(t, b, ctx) // never settled
			time.Sleep(testConfig().ProbeTimeout - time.Nanosecond)
			requireRejected(t, b, ctx, circuit.RejectedError{State: circuit.HalfOpen})
			require.Empty(t, pol.probeCounts())

			time.Sleep(time.Nanosecond)
			mustAllow(t, b, ctx)
			require.Equal(t, []circuit.Counts{{TimedOut: 1}}, pol.probeCounts())
		})
	})

	t.Run("a reopen on a timeout starts the open period when the timeout fell due", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			cfg := testConfig()
			rec := &recorder{}
			pol := &policyRecorder{trip: tripOn(1), probe: reopenOnTimeout}
			b := newBreaker(t, cfg, pol.policy(), classifyByError, rec.observe)
			ctx := t.Context()
			openThenWait(t, b, ctx, cfg)

			mustAllow(t, b, ctx) // never settled
			time.Sleep(cfg.ProbeTimeout + 3*time.Second)

			requireRejected(t, b, ctx, circuit.RejectedError{State: circuit.Open, RetryAfter: cfg.OpenFor - 3*time.Second})
			require.Equal(t, circuit.Transition{
				From: circuit.HalfOpen, To: circuit.Open, Generation: 3,
				Counts: circuit.Counts{TimedOut: 1},
			}, rec.observed()[2])
		})
	})

	t.Run("one call catches up through several overdue changes in order", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			cfg := testConfig()
			rec := &recorder{}
			pol := &policyRecorder{trip: tripOn(1), probe: reopenOnTimeout}
			b := newBreaker(t, cfg, pol.policy(), classifyByError, rec.observe)
			ctx := t.Context()
			openThenWait(t, b, ctx, cfg)

			mustAllow(t, b, ctx) // never settled
			time.Sleep(cfg.ProbeTimeout + cfg.OpenFor + time.Second)

			late := context.WithValue(ctx, ctxKey{}, "late")
			tk := mustAllow(t, b, late)
			require.Equal(t, []circuit.Transition{
				{From: circuit.Closed, To: circuit.Open, Generation: 1, Counts: circuit.Counts{Failures: 1, ConsecutiveFailures: 1}},
				{From: circuit.Open, To: circuit.HalfOpen, Generation: 2},
				{From: circuit.HalfOpen, To: circuit.Open, Generation: 3, Counts: circuit.Counts{TimedOut: 1}},
				{From: circuit.Open, To: circuit.HalfOpen, Generation: 4},
			}, rec.observed())

			// observe receives the context of the first call after each
			// change fell due.
			ctxs := rec.contexts()
			require.Equal(t, "late", ctxs[2].Value(ctxKey{}))
			require.Equal(t, "late", ctxs[3].Value(ctxKey{}))

			require.True(t, tk.Done(nil))
			require.Equal(t, circuit.Closed, b.State())
		})
	})

	t.Run("a slow probe past its timeout still closes the breaker if nothing changed first", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			cfg := testConfig()
			rec := &recorder{}
			pol := &policyRecorder{trip: tripOn(1), probe: closeOnSuccess}
			b := newBreaker(t, cfg, pol.policy(), classifyByError, rec.observe)
			ctx := t.Context()
			openThenWait(t, b, ctx, cfg)

			tk := mustAllow(t, b, ctx)
			time.Sleep(cfg.ProbeTimeout + time.Second)
			require.True(t, tk.Done(nil))

			require.Equal(t, circuit.Closed, b.State())
			require.Equal(t, []circuit.Counts{
				{TimedOut: 1},
				{Successes: 1, ConsecutiveSuccesses: 1, TimedOut: 1},
			}, pol.probeCounts())
			require.Equal(t, circuit.Transition{
				From: circuit.HalfOpen, To: circuit.Closed, Generation: 3,
				Counts: circuit.Counts{Successes: 1, ConsecutiveSuccesses: 1, TimedOut: 1},
			}, rec.observed()[2])
		})
	})

	t.Run("a Release after the deadline leaves the probe to be timed out", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			pol := &policyRecorder{trip: tripOn(1), probe: stayHalfOpen}
			b := newHalfOpen(t, pol, classifyByError)
			ctx := t.Context()

			tk := mustAllow(t, b, ctx)
			time.Sleep(testConfig().ProbeTimeout)
			tk.Release()
			require.Empty(t, pol.probeCounts())

			mustAllow(t, b, ctx)
			require.Equal(t, []circuit.Counts{{TimedOut: 1}}, pol.probeCounts())
		})
	})

	t.Run("probes time out in admission order and settle from any position", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			cfg := testConfig()
			cfg.Probes = 3
			pol := &policyRecorder{trip: tripOn(1), probe: stayHalfOpen}
			b := newBreaker(t, cfg, pol.policy(), classifyByError, ignoreTransitions)
			ctx := t.Context()
			openThenWait(t, b, ctx, cfg)

			// check makes a call that finds every probe that has timed out,
			// and settles it at once, so that it times nothing out itself.
			check := func() int {
				mustAllow(t, b, ctx).Release()
				return len(pol.probeCounts())
			}

			mustAllow(t, b, ctx) // admitted at 0s
			time.Sleep(time.Second)
			middle := mustAllow(t, b, ctx) // admitted at 1s
			time.Sleep(time.Second)
			mustAllow(t, b, ctx) // admitted at 2s
			middle.Release()

			time.Sleep(cfg.ProbeTimeout - 2*time.Second - time.Nanosecond)
			require.Equal(t, 0, check())
			time.Sleep(time.Nanosecond)
			require.Equal(t, 1, check()) // the first probe
			time.Sleep(time.Second)
			require.Equal(t, 1, check()) // the middle probe was settled
			time.Sleep(time.Second)
			require.Equal(t, 2, check()) // the last probe
			require.Equal(t, []circuit.Counts{{TimedOut: 1}, {TimedOut: 2}}, pol.probeCounts())

			// Settle the head, then the tail, then the only probe left.
			head := mustAllow(t, b, ctx)
			only := mustAllow(t, b, ctx)
			tail := mustAllow(t, b, ctx)
			requireRejected(t, b, ctx, circuit.RejectedError{State: circuit.HalfOpen})
			head.Release()
			tail.Release()
			only.Release()

			// Nothing settled is timed out later, and every slot is free.
			time.Sleep(2 * cfg.ProbeTimeout)
			require.Equal(t, 2, check())
			mustAllow(t, b, ctx)
			mustAllow(t, b, ctx)
			mustAllow(t, b, ctx)
			requireRejected(t, b, ctx, circuit.RejectedError{State: circuit.HalfOpen})
		})
	})

	t.Run("Tickets lost while closed cost nothing", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			rec := &recorder{}
			pol := &policyRecorder{trip: tripOn(1), probe: closeOnSuccess}
			b := newBreaker(t, testConfig(), pol.policy(), classifyByError, rec.observe)
			ctx := t.Context()

			for range 1000 {
				mustAllow(t, b, ctx) // never settled
			}
			time.Sleep(24 * time.Hour)

			require.Equal(t, circuit.Closed, b.State())
			require.True(t, settle(t, b, ctx, nil))
			require.Equal(t, []circuit.Counts{{Successes: 1, ConsecutiveSuccesses: 1}}, pol.tripCounts())
			require.Empty(t, pol.probeCounts())
			require.Empty(t, rec.observed())
		})
	})
}
