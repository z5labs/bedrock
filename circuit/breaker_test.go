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

func TestNew(t *testing.T) {
	t.Parallel()

	valid := circuit.Policy{Trip: neverTrip, Probe: stayHalfOpen}

	t.Run("panics on a nil function before checking Config", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name     string
			policy   circuit.Policy
			classify func(context.Context, error) circuit.Outcome
			observe  func(context.Context, circuit.Transition)
			want     string
		}{
			{
				name:     "nil Trip",
				policy:   circuit.Policy{Probe: valid.Probe},
				classify: classifyByError,
				observe:  ignoreTransitions,
				want:     "circuit: New called with a nil Policy.Trip",
			},
			{
				name:     "nil Probe",
				policy:   circuit.Policy{Trip: valid.Trip},
				classify: classifyByError,
				observe:  ignoreTransitions,
				want:     "circuit: New called with a nil Policy.Probe",
			},
			{
				name:    "nil classify",
				policy:  valid,
				observe: ignoreTransitions,
				want:    "circuit: New called with a nil classify function",
			},
			{
				name:     "nil observe",
				policy:   valid,
				classify: classifyByError,
				want:     "circuit: New called with a nil observe function",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				// The zero Config is invalid, so a panic shows that the
				// functions are checked first.
				require.PanicsWithValue(t, tc.want, func() {
					_, _ = circuit.New(circuit.Config{}, tc.policy, tc.classify, tc.observe)
				})
			})
		}
	})

	t.Run("returns an error naming an invalid field", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name   string
			modify func(*circuit.Config)
			want   string
		}{
			{
				name:   "window of 0",
				modify: func(c *circuit.Config) { c.Window = 0 },
				want:   "circuit: window must be between 1 and 65536, got 0",
			},
			{
				name:   "window of -1",
				modify: func(c *circuit.Config) { c.Window = -1 },
				want:   "circuit: window must be between 1 and 65536, got -1",
			},
			{
				name:   "window of MaxCount+1",
				modify: func(c *circuit.Config) { c.Window = circuit.MaxCount + 1 },
				want:   "circuit: window must be between 1 and 65536, got 65537",
			},
			{
				name:   "probes of 0",
				modify: func(c *circuit.Config) { c.Probes = 0 },
				want:   "circuit: probes must be between 1 and 65536, got 0",
			},
			{
				name:   "probes of -1",
				modify: func(c *circuit.Config) { c.Probes = -1 },
				want:   "circuit: probes must be between 1 and 65536, got -1",
			},
			{
				name:   "probes of MaxCount+1",
				modify: func(c *circuit.Config) { c.Probes = circuit.MaxCount + 1 },
				want:   "circuit: probes must be between 1 and 65536, got 65537",
			},
			{
				name:   "open duration of 0",
				modify: func(c *circuit.Config) { c.OpenFor = 0 },
				want:   "circuit: open duration must be positive, got 0s",
			},
			{
				name:   "negative open duration",
				modify: func(c *circuit.Config) { c.OpenFor = -time.Nanosecond },
				want:   "circuit: open duration must be positive, got -1ns",
			},
			{
				name:   "probe timeout of 0",
				modify: func(c *circuit.Config) { c.ProbeTimeout = 0 },
				want:   "circuit: probe timeout must be positive, got 0s",
			},
			{
				name:   "negative probe timeout",
				modify: func(c *circuit.Config) { c.ProbeTimeout = -time.Nanosecond },
				want:   "circuit: probe timeout must be positive, got -1ns",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				cfg := testConfig()
				tc.modify(&cfg)

				b, err := circuit.New(cfg, valid, classifyByError, ignoreTransitions)
				require.Nil(t, b)
				require.EqualError(t, err, tc.want)
			})
		}
	})

	t.Run("accepts the bounds", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name string
			cfg  circuit.Config
		}{
			{
				name: "lower bounds",
				cfg:  circuit.Config{Window: 1, Probes: 1, OpenFor: time.Nanosecond, ProbeTimeout: time.Nanosecond},
			},
			{
				name: "upper bounds",
				cfg:  circuit.Config{Window: circuit.MaxCount, Probes: circuit.MaxCount, OpenFor: time.Hour, ProbeTimeout: time.Hour},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				b := newBreaker(t, tc.cfg, valid, classifyByError, ignoreTransitions)
				require.Equal(t, circuit.Closed, b.State())
			})
		}
	})

	t.Run("names every invalid field of a zero Config in one error", func(t *testing.T) {
		t.Parallel()

		b, err := circuit.New(circuit.Config{}, valid, classifyByError, ignoreTransitions)
		require.Nil(t, b)
		require.EqualError(t, err, "circuit: window must be between 1 and 65536, got 0\n"+
			"circuit: probes must be between 1 and 65536, got 0\n"+
			"circuit: open duration must be positive, got 0s\n"+
			"circuit: probe timeout must be positive, got 0s")
	})
}

func TestBreaker_argumentAssertions(t *testing.T) {
	t.Parallel()

	t.Run("Allow with a nil context panics", func(t *testing.T) {
		t.Parallel()

		b := newBreaker(t, testConfig(), circuit.Policy{Trip: neverTrip, Probe: stayHalfOpen}, classifyByError, ignoreTransitions)

		var nilCtx context.Context
		require.PanicsWithValue(t, "circuit: Breaker.Allow called with a nil context", func() {
			_, _ = b.Allow(nilCtx)
		})
	})

	testCases := []struct {
		name string
		b    *circuit.Breaker
	}{
		{name: "nil Breaker", b: nil},
		{name: "zero Breaker", b: &circuit.Breaker{}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.PanicsWithValue(t, "circuit: Breaker.Allow called on a Breaker not built by New", func() {
				_, _ = tc.b.Allow(t.Context())
			})
			require.PanicsWithValue(t, "circuit: Breaker.State called on a Breaker not built by New", func() {
				_ = tc.b.State()
			})
		})
	}
}

func TestBreaker_lifecycle(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		cfg := circuit.Config{Window: 10, Probes: 2, OpenFor: 10 * time.Second, ProbeTimeout: 5 * time.Second}
		policy := circuit.Policy{
			Trip: tripOn(3),
			Probe: func(c circuit.Counts) circuit.State {
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
		rec := &recorder{}
		b := newBreaker(t, cfg, policy, classifyByError, rec.observe)
		ctx := t.Context()

		// closed → open on Trip
		require.Equal(t, circuit.Closed, b.State())
		require.True(t, settle(t, b, ctx, nil))
		require.True(t, settle(t, b, ctx, errFailure))
		require.True(t, settle(t, b, ctx, errFailure))
		require.Empty(t, rec.observed())
		require.True(t, settle(t, b, ctx, errFailure))
		require.Equal(t, []circuit.Transition{
			{From: circuit.Closed, To: circuit.Open, Generation: 1, Counts: circuit.Counts{Successes: 1, Failures: 3, ConsecutiveFailures: 3}},
		}, rec.observed())
		require.Equal(t, circuit.Open, b.State())

		// rejected while open, with RetryAfter counting down
		requireRejected(t, b, ctx, circuit.RejectedError{State: circuit.Open, RetryAfter: 10 * time.Second})
		time.Sleep(4 * time.Second)
		requireRejected(t, b, ctx, circuit.RejectedError{State: circuit.Open, RetryAfter: 6 * time.Second})
		time.Sleep(6*time.Second - time.Nanosecond)
		require.Equal(t, circuit.Open, b.State())
		requireRejected(t, b, ctx, circuit.RejectedError{State: circuit.Open, RetryAfter: time.Nanosecond})

		// State reports half-open at exactly OpenFor, without a Transition
		time.Sleep(time.Nanosecond)
		require.Equal(t, circuit.HalfOpen, b.State())
		require.Len(t, rec.observed(), 1)

		// the next call makes open → half-open
		p1 := mustAllow(t, b, ctx)
		require.Equal(t, circuit.Transition{From: circuit.Open, To: circuit.HalfOpen, Generation: 2}, rec.observed()[1])
		p2 := mustAllow(t, b, ctx)

		// the probe cap
		requireRejected(t, b, ctx, circuit.RejectedError{State: circuit.HalfOpen})

		// half-open → closed
		require.True(t, p1.Done(nil))
		require.Len(t, rec.observed(), 2)
		require.True(t, p2.Done(nil))
		require.Equal(t, circuit.Transition{
			From: circuit.HalfOpen, To: circuit.Closed, Generation: 3,
			Counts: circuit.Counts{Successes: 2, ConsecutiveSuccesses: 2},
		}, rec.observed()[2])
		require.Equal(t, circuit.Closed, b.State())

		// half-open → open on a failed probe starts a new open period
		require.True(t, settle(t, b, ctx, errFailure))
		require.True(t, settle(t, b, ctx, errFailure))
		require.True(t, settle(t, b, ctx, errFailure))
		require.Equal(t, circuit.Transition{
			From: circuit.Closed, To: circuit.Open, Generation: 4,
			Counts: circuit.Counts{Failures: 3, ConsecutiveFailures: 3},
		}, rec.observed()[3])
		time.Sleep(cfg.OpenFor)
		p3 := mustAllow(t, b, ctx)
		require.Equal(t, circuit.Transition{From: circuit.Open, To: circuit.HalfOpen, Generation: 5}, rec.observed()[4])
		require.True(t, p3.Done(errFailure))
		require.Equal(t, circuit.Transition{
			From: circuit.HalfOpen, To: circuit.Open, Generation: 6,
			Counts: circuit.Counts{Failures: 1, ConsecutiveFailures: 1},
		}, rec.observed()[5])
		require.Equal(t, circuit.Open, b.State())
		requireRejected(t, b, ctx, circuit.RejectedError{State: circuit.Open, RetryAfter: cfg.OpenFor})
		time.Sleep(cfg.OpenFor - time.Nanosecond)
		require.Equal(t, circuit.Open, b.State())

		require.Len(t, rec.observed(), 6)
		require.False(t, rec.overlapped.Load())
	})
}
