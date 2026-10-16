// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package circuit_test

import (
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/z5labs/bedrock/circuit"
)

const (
	settledByDone    = "circuit: Ticket.Done called on a Ticket already settled by Done"
	settledByRelease = "circuit: Ticket.Done called on a Ticket already settled by Release"
)

func TestTicket(t *testing.T) {
	t.Parallel()

	t.Run("Done twice panics and counts once", func(t *testing.T) {
		t.Parallel()

		pol := &policyRecorder{trip: neverTrip, probe: stayHalfOpen}
		b := newBreaker(t, testConfig(), pol.policy(), classifyByError, ignoreTransitions)

		tk := mustAllow(t, b, t.Context())
		require.True(t, tk.Done(errFailure))
		require.PanicsWithValue(t, settledByDone, func() { tk.Done(errFailure) })

		require.Equal(t, []circuit.Counts{{Failures: 1, ConsecutiveFailures: 1}}, pol.tripCounts())
	})

	t.Run("Release then Done panics and counts nothing", func(t *testing.T) {
		t.Parallel()

		pol := &policyRecorder{trip: neverTrip, probe: stayHalfOpen}
		b := newBreaker(t, testConfig(), pol.policy(), classifyByError, ignoreTransitions)

		tk := mustAllow(t, b, t.Context())
		tk.Release()
		require.PanicsWithValue(t, settledByRelease, func() { tk.Done(errFailure) })

		require.Empty(t, pol.tripCounts())
	})

	t.Run("Release twice does nothing and does not free a newer probe's slot", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			cfg := testConfig()
			pol := &policyRecorder{trip: tripOn(1), probe: closeOnSuccess}
			b := newBreaker(t, cfg, pol.policy(), classifyByError, ignoreTransitions)
			ctx := t.Context()
			openThenWait(t, b, ctx, cfg)

			old := mustAllow(t, b, ctx)
			old.Release()
			newer := mustAllow(t, b, ctx)
			old.Release()

			requireRejected(t, b, ctx, circuit.RejectedError{State: circuit.HalfOpen})
			require.Empty(t, pol.probeCounts())

			newer.Release()
			requireNextProbeCloses(t, b, ctx)
		})
	})

	t.Run("Done then Release does nothing", func(t *testing.T) {
		t.Parallel()

		t.Run("closed", func(t *testing.T) {
			t.Parallel()

			pol := &policyRecorder{trip: neverTrip, probe: stayHalfOpen}
			b := newBreaker(t, testConfig(), pol.policy(), classifyByError, ignoreTransitions)

			tk := mustAllow(t, b, t.Context())
			require.True(t, tk.Done(errFailure))
			tk.Release()
			require.True(t, settle(t, b, t.Context(), errFailure))

			require.Equal(t, []circuit.Counts{
				{Failures: 1, ConsecutiveFailures: 1},
				{Failures: 2, ConsecutiveFailures: 2},
			}, pol.tripCounts())
		})

		t.Run("half-open", func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				cfg := testConfig()
				cfg.Probes = 2
				pol := &policyRecorder{trip: tripOn(1), probe: stayHalfOpen}
				b := newBreaker(t, cfg, pol.policy(), classifyByError, ignoreTransitions)
				ctx := t.Context()
				openThenWait(t, b, ctx, cfg)

				p1 := mustAllow(t, b, ctx)
				p2 := mustAllow(t, b, ctx)
				require.True(t, p1.Done(nil))
				p1.Release()

				// Release freed nothing a second time: one slot is free,
				// not two.
				p3 := mustAllow(t, b, ctx)
				requireRejected(t, b, ctx, circuit.RejectedError{State: circuit.HalfOpen})

				require.True(t, p2.Done(nil))
				require.True(t, p3.Done(nil))
				require.Equal(t, []circuit.Counts{
					{Successes: 1, ConsecutiveSuccesses: 1},
					{Successes: 2, ConsecutiveSuccesses: 2},
					{Successes: 3, ConsecutiveSuccesses: 3},
				}, pol.probeCounts())
			})
		})
	})

	t.Run("a Ticket handed off past a plain deferred Release panics when its result arrives", func(t *testing.T) {
		t.Parallel()

		b := newBreaker(t, testConfig(), circuit.Policy{Trip: neverTrip, Probe: stayHalfOpen}, classifyByError, ignoreTransitions)

		handOff := func() *circuit.Ticket {
			tk := mustAllow(t, b, t.Context())
			defer tk.Release()
			return tk
		}

		tk := handOff()
		require.PanicsWithValue(t, settledByRelease, func() { tk.Done(nil) })
	})

	t.Run("a Ticket not returned by Allow panics", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name string
			tk   *circuit.Ticket
		}{
			{name: "nil Ticket", tk: nil},
			{name: "zero Ticket", tk: &circuit.Ticket{}},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				require.PanicsWithValue(t, "circuit: Ticket.Done called on a Ticket not returned by Breaker.Allow", func() {
					tc.tk.Done(nil)
				})
				require.PanicsWithValue(t, "circuit: Ticket.Release called on a Ticket not returned by Breaker.Allow", func() {
					tc.tk.Release()
				})
			})
		}
	})

	t.Run("a Ticket may be settled on another goroutine", func(t *testing.T) {
		t.Parallel()

		pol := &policyRecorder{trip: neverTrip, probe: stayHalfOpen}
		b := newBreaker(t, testConfig(), pol.policy(), classifyByError, ignoreTransitions)

		tk := mustAllow(t, b, t.Context())
		counted := make(chan bool)
		go func() { counted <- tk.Done(errFailure) }()

		require.True(t, <-counted)
		require.Equal(t, []circuit.Counts{{Failures: 1, ConsecutiveFailures: 1}}, pol.tripCounts())
	})

	t.Run("racing Done and Done settle exactly once", func(t *testing.T) {
		t.Parallel()

		pol := &policyRecorder{trip: neverTrip, probe: stayHalfOpen}
		b := newBreaker(t, testConfig(), pol.policy(), classifyByError, ignoreTransitions)

		const rounds = 100
		for range rounds {
			tk := mustAllow(t, b, t.Context())

			var wg sync.WaitGroup
			var panics [2]any
			for i := range panics {
				wg.Go(func() { panics[i] = catchPanic(func() { tk.Done(nil) }) })
			}
			wg.Wait()

			require.ElementsMatch(t, []any{nil, settledByDone}, panics[:])
		}

		require.Len(t, pol.tripCounts(), rounds)
	})

	t.Run("racing Done and Release settle exactly once", func(t *testing.T) {
		t.Parallel()

		pol := &policyRecorder{trip: neverTrip, probe: stayHalfOpen}
		b := newBreaker(t, testConfig(), pol.policy(), classifyByError, ignoreTransitions)

		const rounds = 100
		doneWon := 0
		for range rounds {
			tk := mustAllow(t, b, t.Context())

			var wg sync.WaitGroup
			var donePanic any
			wg.Go(func() { donePanic = catchPanic(func() { tk.Done(nil) }) })
			wg.Go(tk.Release)
			wg.Wait()

			switch donePanic {
			case nil:
				doneWon++
			default:
				require.Equal(t, settledByRelease, donePanic)
			}
		}

		require.Len(t, pol.tripCounts(), doneWon)
	})
}

func TestTicket_lateResults(t *testing.T) {
	t.Parallel()

	t.Run("a result admitted while closed is discarded once the state has changed", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			cfg := testConfig()
			pol := &policyRecorder{trip: tripOn(1), probe: closeOnSuccess}
			b := newBreaker(t, cfg, pol.policy(), classifyByError, ignoreTransitions)
			ctx := t.Context()

			lateWhileOpen := mustAllow(t, b, ctx)
			lateWhileHalfOpen := mustAllow(t, b, ctx)
			require.True(t, settle(t, b, ctx, errFailure))
			require.Equal(t, circuit.Open, b.State())

			// settled while open
			require.False(t, lateWhileOpen.Done(errFailure))
			require.Len(t, pol.tripCounts(), 1)
			require.Empty(t, pol.probeCounts())
			require.Equal(t, circuit.Open, b.State())

			// settled while half-open
			time.Sleep(cfg.OpenFor)
			probe := mustAllow(t, b, ctx)
			require.False(t, lateWhileHalfOpen.Done(nil))
			require.Len(t, pol.tripCounts(), 1)
			require.Empty(t, pol.probeCounts())
			require.Equal(t, circuit.HalfOpen, b.State())
			requireRejected(t, b, ctx, circuit.RejectedError{State: circuit.HalfOpen})

			require.True(t, probe.Done(nil))
			require.Equal(t, circuit.Closed, b.State())
		})
	})

	t.Run("an old probe whose slot was reused after a change of state neither frees nor counts in it", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			cfg := testConfig()
			pol := &policyRecorder{
				trip: tripOn(1),
				probe: func(c circuit.Counts) circuit.State {
					if c.TimedOut > 0 {
						return circuit.Open
					}
					return closeOnSuccess(c)
				},
			}
			b := newBreaker(t, cfg, pol.policy(), classifyByError, ignoreTransitions)
			ctx := t.Context()
			openThenWait(t, b, ctx, cfg)

			old := mustAllow(t, b, ctx)
			time.Sleep(cfg.ProbeTimeout)
			requireRejected(t, b, ctx, circuit.RejectedError{State: circuit.Open, RetryAfter: cfg.OpenFor})
			time.Sleep(cfg.OpenFor)
			newer := mustAllow(t, b, ctx) // reuses the only slot
			require.Equal(t, []circuit.Counts{{TimedOut: 1}}, pol.probeCounts())

			require.False(t, old.Done(nil))
			old.Release()
			require.Len(t, pol.probeCounts(), 1)
			requireRejected(t, b, ctx, circuit.RejectedError{State: circuit.HalfOpen})

			require.True(t, newer.Done(nil))
			require.Equal(t, circuit.Closed, b.State())
		})
	})

	t.Run("an old probe whose slot was reused in the same state counts but does not free it", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			cfg := testConfig()
			pol := &policyRecorder{
				trip: tripOn(1),
				probe: func(c circuit.Counts) circuit.State {
					if c.Successes >= 2 {
						return circuit.Closed
					}
					return circuit.HalfOpen
				},
			}
			b := newBreaker(t, cfg, pol.policy(), classifyByError, ignoreTransitions)
			ctx := t.Context()
			openThenWait(t, b, ctx, cfg)

			old := mustAllow(t, b, ctx)
			time.Sleep(cfg.ProbeTimeout)
			newer := mustAllow(t, b, ctx) // old timed out, and its slot is reused

			// A probe that timed out still counts until the state changes.
			require.True(t, old.Done(nil))
			requireRejected(t, b, ctx, circuit.RejectedError{State: circuit.HalfOpen})

			require.True(t, newer.Done(nil))
			require.Equal(t, circuit.Closed, b.State())
			require.Equal(t, []circuit.Counts{
				{TimedOut: 1},
				{Successes: 1, ConsecutiveSuccesses: 1, TimedOut: 1},
				{Successes: 2, ConsecutiveSuccesses: 2, TimedOut: 1},
			}, pol.probeCounts())
		})
	})
}
