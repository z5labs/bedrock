// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package circuit_test

import (
	"context"
	"runtime"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/z5labs/bedrock/circuit"
)

// newHalfOpen returns a Breaker with one probe slot whose open period has
// passed, so that the next call to Allow makes it half-open. It must be
// called inside a synctest bubble.
func newHalfOpen(t *testing.T, pol *policyRecorder, classify func(context.Context, error) circuit.Outcome) *circuit.Breaker {
	t.Helper()

	cfg := testConfig()
	b := newBreaker(t, cfg, pol.policy(), classify, ignoreTransitions)
	openThenWait(t, b, t.Context(), cfg)
	return b
}

func TestBreaker_panics(t *testing.T) {
	t.Parallel()

	t.Run("code between Allow and Done panics with only the deferred Release", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			pol := &policyRecorder{trip: tripOn(1), probe: closeOnSuccess}
			b := newHalfOpen(t, pol, classifyByError)
			ctx := t.Context()

			require.PanicsWithValue(t, "boom", func() {
				tk := mustAllow(t, b, ctx)
				defer tk.Release()
				panic("boom")
			})

			require.Empty(t, pol.probeCounts())
			requireNextProbeCloses(t, b, ctx)
		})
	})

	t.Run("runtime.Goexit between Allow and Done", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			pol := &policyRecorder{trip: tripOn(1), probe: closeOnSuccess}
			b := newHalfOpen(t, pol, classifyByError)
			ctx := t.Context()

			var allowErr error
			exited := make(chan struct{})
			go func() {
				defer close(exited)

				tk, err := b.Allow(ctx)
				if err != nil {
					allowErr = err
					return
				}
				defer tk.Release()
				runtime.Goexit()
			}()
			<-exited

			require.NoError(t, allowErr)
			require.Empty(t, pol.probeCounts())
			requireNextProbeCloses(t, b, ctx)
		})
	})

	t.Run("the classifier fails inside Done", func(t *testing.T) {
		t.Parallel()

		classifiers := []struct {
			name     string
			classify func(context.Context, error) circuit.Outcome
			want     any
		}{
			{
				name:     "panics",
				classify: func(context.Context, error) circuit.Outcome { panic("classifier boom") },
				want:     "classifier boom",
			},
			{
				name:     "returns Outcome(0)",
				classify: func(context.Context, error) circuit.Outcome { return circuit.Outcome(0) },
				want:     "circuit: the classifier returned Outcome(0), which is not an Outcome",
			},
			{
				name:     "returns Outcome(4)",
				classify: func(context.Context, error) circuit.Outcome { return circuit.Outcome(4) },
				want:     "circuit: the classifier returned Outcome(4), which is not an Outcome",
			},
		}

		for _, c := range classifiers {
			for _, deferred := range []bool{false, true} {
				name := c.name + " without a deferred Release"
				if deferred {
					name = c.name + " with a deferred Release"
				}

				t.Run(name, func(t *testing.T) {
					t.Parallel()

					synctest.Test(t, func(t *testing.T) {
						sw := &switchClassifier{fn: classifyByError}
						pol := &policyRecorder{trip: tripOn(1), probe: closeOnSuccess}
						b := newHalfOpen(t, pol, sw.classify)
						ctx := t.Context()

						sw.set(c.classify)
						require.PanicsWithValue(t, c.want, func() {
							tk := mustAllow(t, b, ctx)
							if deferred {
								defer tk.Release()
							}
							tk.Done(nil)
						})
						sw.set(classifyByError)

						require.Empty(t, pol.probeCounts())
						require.Equal(t, circuit.HalfOpen, b.State())
						requireNextProbeCloses(t, b, ctx)
					})
				})
			}
		}
	})

	t.Run("a Trip panic keeps what was counted", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			cfg := testConfig()
			rec := &recorder{}
			pol := &policyRecorder{
				trip:  func(circuit.Counts) bool { panic("trip boom") },
				probe: closeOnSuccess,
			}
			b := newBreaker(t, cfg, pol.policy(), classifyByError, rec.observe)
			ctx := t.Context()

			require.PanicsWithValue(t, "trip boom", func() { settle(t, b, ctx, errFailure) })
			require.Equal(t, circuit.Closed, b.State())

			pol.setTrip(tripOn(2))
			require.True(t, settle(t, b, ctx, errFailure))
			require.Equal(t, circuit.Open, b.State())
			require.Equal(t, []circuit.Transition{
				{From: circuit.Closed, To: circuit.Open, Generation: 1, Counts: circuit.Counts{Failures: 2, ConsecutiveFailures: 2}},
			}, rec.observed())

			time.Sleep(cfg.OpenFor)
			requireNextProbeCloses(t, b, ctx)
		})
	})

	t.Run("Probe fails, freeing the slot and staying half-open", func(t *testing.T) {
		t.Parallel()

		probes := []struct {
			name  string
			probe func(circuit.Counts) circuit.State
			want  any
		}{
			{
				name:  "panics",
				probe: func(circuit.Counts) circuit.State { panic("probe boom") },
				want:  "probe boom",
			},
			{
				name:  "returns State(0)",
				probe: func(circuit.Counts) circuit.State { return circuit.State(0) },
				want:  "circuit: Policy.Probe returned State(0), which is not a State",
			},
			{
				name:  "returns State(4)",
				probe: func(circuit.Counts) circuit.State { return circuit.State(4) },
				want:  "circuit: Policy.Probe returned State(4), which is not a State",
			},
		}

		for _, p := range probes {
			t.Run(p.name, func(t *testing.T) {
				t.Parallel()

				synctest.Test(t, func(t *testing.T) {
					pol := &policyRecorder{trip: tripOn(1), probe: closeOnSuccess}
					b := newHalfOpen(t, pol, classifyByError)
					ctx := t.Context()

					pol.setProbe(p.probe)
					require.PanicsWithValue(t, p.want, func() { settle(t, b, ctx, nil) })
					pol.setProbe(closeOnSuccess)

					require.Equal(t, circuit.HalfOpen, b.State())
					requireNextProbeCloses(t, b, ctx)
					require.Equal(t, []circuit.Counts{
						{Successes: 1, ConsecutiveSuccesses: 1},
						{Successes: 2, ConsecutiveSuccesses: 2},
					}, pol.probeCounts())
				})
			})
		}
	})

	t.Run("a Probe panic during catch-up in Allow admits nothing and leaks no slot", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			pol := &policyRecorder{trip: tripOn(1), probe: closeOnSuccess}
			b := newHalfOpen(t, pol, classifyByError)
			ctx := t.Context()

			mustAllow(t, b, ctx) // never settled
			time.Sleep(testConfig().ProbeTimeout)

			pol.setProbe(func(circuit.Counts) circuit.State { panic("probe boom") })
			var admitted *circuit.Ticket
			require.PanicsWithValue(t, "probe boom", func() { admitted, _ = b.Allow(ctx) })
			require.Nil(t, admitted)
			pol.setProbe(closeOnSuccess)

			requireNextProbeCloses(t, b, ctx)
			require.Equal(t, []circuit.Counts{
				{TimedOut: 1},
				{Successes: 1, ConsecutiveSuccesses: 1, TimedOut: 1},
			}, pol.probeCounts())
		})
	})
}

func TestBreaker_earlyReturn(t *testing.T) {
	t.Parallel()

	// call returns early, before Done, when prepare fails.
	call := func(b *circuit.Breaker, ctx context.Context, prepare func() error) error {
		tk, err := b.Allow(ctx)
		if err != nil {
			return err
		}
		defer tk.Release()

		if err := prepare(); err != nil {
			return err
		}
		tk.Done(nil)
		return nil
	}
	failToPrepare := func() error { return errFailure }

	t.Run("a call settled only by the deferred Release counts nothing and frees its probe slot", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			pol := &policyRecorder{trip: tripOn(1), probe: closeOnSuccess}
			b := newHalfOpen(t, pol, classifyByError)
			ctx := t.Context()

			require.ErrorIs(t, call(b, ctx, failToPrepare), errFailure)

			require.Empty(t, pol.probeCounts())
			require.Equal(t, circuit.HalfOpen, b.State())
			requireNextProbeCloses(t, b, ctx)
		})
	})

	t.Run("failures returned before Done never call Trip", func(t *testing.T) {
		t.Parallel()

		// This is the documented footgun: a result is counted only by Done.
		pol := &policyRecorder{trip: tripOn(1), probe: closeOnSuccess}
		b := newBreaker(t, testConfig(), pol.policy(), classifyByError, ignoreTransitions)

		for range 100 {
			require.ErrorIs(t, call(b, t.Context(), failToPrepare), errFailure)
		}

		require.Empty(t, pol.tripCounts())
		require.Equal(t, circuit.Closed, b.State())
	})
}
