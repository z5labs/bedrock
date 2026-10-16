// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package circuit_test

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/z5labs/bedrock/circuit"
)

// The runs saturate at math.MaxInt rather than wrap. That is checked by
// inspecting increment in breaker.go, since exercising it would take about
// 2^63 results.

func TestBreaker_counts(t *testing.T) {
	t.Parallel()

	t.Run("the ring evicts the oldest result", func(t *testing.T) {
		t.Parallel()

		pol := &policyRecorder{trip: neverTrip, probe: stayHalfOpen}
		cfg := testConfig()
		cfg.Window = 3
		b := newBreaker(t, cfg, pol.policy(), classifyByError, ignoreTransitions)

		for _, err := range []error{errFailure, nil, nil, nil, errFailure} {
			require.True(t, settle(t, b, t.Context(), err))
		}

		require.Equal(t, []circuit.Counts{
			{Failures: 1, ConsecutiveFailures: 1},
			{Successes: 1, Failures: 1, ConsecutiveSuccesses: 1},
			{Successes: 2, Failures: 1, ConsecutiveSuccesses: 2},
			{Successes: 3, ConsecutiveSuccesses: 3},             // the failure is evicted
			{Successes: 2, Failures: 1, ConsecutiveFailures: 1}, // a success is evicted
		}, pol.tripCounts())
	})

	t.Run("a run longer than Window still trips", func(t *testing.T) {
		t.Parallel()

		rec := &recorder{}
		pol := &policyRecorder{trip: tripOn(5), probe: stayHalfOpen}
		cfg := testConfig()
		cfg.Window = 3
		b := newBreaker(t, cfg, pol.policy(), classifyByError, rec.observe)

		for range 4 {
			require.True(t, settle(t, b, t.Context(), errFailure))
			require.Equal(t, circuit.Closed, b.State())
		}
		require.True(t, settle(t, b, t.Context(), errFailure))

		require.Equal(t, circuit.Open, b.State())
		require.Equal(t, []circuit.Transition{
			{From: circuit.Closed, To: circuit.Open, Generation: 1, Counts: circuit.Counts{Failures: 3, ConsecutiveFailures: 5}},
		}, rec.observed())
	})

	t.Run("Ignored neither extends nor ends a run and does not call Trip", func(t *testing.T) {
		t.Parallel()

		pol := &policyRecorder{trip: neverTrip, probe: stayHalfOpen}
		b := newBreaker(t, testConfig(), pol.policy(), classifyByError, ignoreTransitions)

		for _, err := range []error{errFailure, errIgnored, errFailure, nil, errIgnored, nil} {
			require.True(t, settle(t, b, t.Context(), err))
		}

		require.Equal(t, []circuit.Counts{
			{Failures: 1, ConsecutiveFailures: 1},
			{Failures: 2, ConsecutiveFailures: 2},
			{Successes: 1, Failures: 2, ConsecutiveSuccesses: 1},
			{Successes: 2, Failures: 2, ConsecutiveSuccesses: 2},
		}, pol.tripCounts())
	})

	t.Run("every change of state resets them", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			rec := &recorder{}
			pol := &policyRecorder{
				trip: tripOn(2),
				probe: func(c circuit.Counts) circuit.State {
					switch {
					case c.Failures > 0:
						return circuit.Open
					case c.Successes >= 2:
						return circuit.Closed
					default:
						return circuit.HalfOpen
					}
				},
			}
			cfg := testConfig()
			b := newBreaker(t, cfg, pol.policy(), classifyByError, rec.observe)
			ctx := t.Context()

			settle(t, b, ctx, nil)
			settle(t, b, ctx, errFailure)
			settle(t, b, ctx, errFailure)
			require.Equal(t, circuit.Open, b.State())

			// half-open starts from zero
			time.Sleep(cfg.OpenFor)
			settle(t, b, ctx, nil)
			settle(t, b, ctx, nil)
			require.Equal(t, circuit.Closed, b.State())
			require.Equal(t, []circuit.Counts{
				{Successes: 1, ConsecutiveSuccesses: 1},
				{Successes: 2, ConsecutiveSuccesses: 2},
			}, pol.probeCounts())

			// closed starts from zero again
			settle(t, b, ctx, errFailure)
			require.Equal(t, circuit.Counts{Failures: 1, ConsecutiveFailures: 1}, pol.tripCounts()[3])
			settle(t, b, ctx, errFailure)

			// and so does half-open after a reopen
			time.Sleep(cfg.OpenFor)
			settle(t, b, ctx, errFailure)
			time.Sleep(cfg.OpenFor)
			settle(t, b, ctx, nil)
			require.Equal(t, circuit.Counts{Successes: 1, ConsecutiveSuccesses: 1}, pol.probeCounts()[3])

			var got []circuit.Counts
			for _, tr := range rec.observed() {
				got = append(got, tr.Counts)
			}
			require.Equal(t, []circuit.Counts{
				{Successes: 1, Failures: 2, ConsecutiveFailures: 2},
				{},
				{Successes: 2, ConsecutiveSuccesses: 2},
				{Failures: 2, ConsecutiveFailures: 2},
				{},
				{Failures: 1, ConsecutiveFailures: 1},
				{},
			}, got)
		})
	})
}
