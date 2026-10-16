// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package circuit_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/z5labs/bedrock/circuit"
)

func TestBreaker_observer(t *testing.T) {
	t.Parallel()

	t.Run("delivers in order, without gaps and never concurrently", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			cfg := testConfig()
			rec := &recorder{}
			b := newBreaker(t, cfg, circuit.Policy{Trip: tripOn(1), Probe: closeOnSuccess}, classifyByError, rec.observe)
			ctx := t.Context()

			for range 3 {
				settle(t, b, ctx, errFailure)
				time.Sleep(cfg.OpenFor)
				settle(t, b, ctx, errFailure)
				time.Sleep(cfg.OpenFor)
				settle(t, b, ctx, nil)
			}

			got := rec.observed()
			require.Len(t, got, 15)
			from := circuit.Closed
			for i, tr := range got {
				require.Equal(t, uint64(i+1), tr.Generation)
				require.Equal(t, from, tr.From)
				from = tr.To
			}
			require.Equal(t, circuit.Closed, from)
			require.False(t, rec.overlapped.Load())
		})
	})

	t.Run("an observer may call State and a nested Allow and Done", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			cfg := testConfig()
			rec := &recorder{}
			var (
				b         *circuit.Breaker
				depth     atomic.Int32
				reentered atomic.Bool
				states    []circuit.State
				nestedErr error
				nested    bool
			)
			observe := func(ctx context.Context, tr circuit.Transition) {
				if depth.Add(1) != 1 {
					reentered.Store(true)
				}
				defer depth.Add(-1)

				rec.observe(ctx, tr)
				states = append(states, b.State())
				if tr.To == circuit.HalfOpen && !nested {
					nested = true
					var tk *circuit.Ticket
					tk, nestedErr = b.Allow(ctx)
					if nestedErr == nil {
						tk.Done(nil) // closes the breaker
					}
				}
			}
			b = newBreaker(t, cfg, circuit.Policy{Trip: tripOn(1), Probe: closeOnSuccess}, classifyByError, observe)
			ctx := t.Context()
			openThenWait(t, b, ctx, cfg)

			tk := mustAllow(t, b, ctx)

			require.NoError(t, nestedErr)
			require.False(t, reentered.Load())
			require.Equal(t, []circuit.Transition{
				{From: circuit.Closed, To: circuit.Open, Generation: 1, Counts: circuit.Counts{Failures: 1, ConsecutiveFailures: 1}},
				{From: circuit.Open, To: circuit.HalfOpen, Generation: 2},
				{From: circuit.HalfOpen, To: circuit.Closed, Generation: 3, Counts: circuit.Counts{Successes: 1, ConsecutiveSuccesses: 1}},
			}, rec.observed())
			require.Equal(t, []circuit.State{circuit.Open, circuit.HalfOpen, circuit.Closed}, states)

			// The outer call is admitted into the state the observer left.
			require.True(t, tk.Done(nil))
			require.Equal(t, circuit.Closed, b.State())
		})
	})

	t.Run("a panic leaves the rest of the queue for the next call, which delivers it before taking the only probe slot", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			cfg := testConfig()
			rec := &recorder{}
			var (
				b         *circuit.Breaker
				nestedErr error
			)
			observe := func(ctx context.Context, tr circuit.Transition) {
				rec.observe(ctx, tr)
				switch tr.Generation {
				case 3:
					panic("observe boom")
				case 4:
					// The call that delivers this has not taken the only
					// probe slot yet.
					var tk *circuit.Ticket
					tk, nestedErr = b.Allow(ctx)
					if nestedErr == nil {
						tk.Release()
					}
				}
			}
			b = newBreaker(t, cfg, circuit.Policy{Trip: tripOn(1), Probe: reopenOnTimeout}, classifyByError, observe)
			ctx := t.Context()
			openThenWait(t, b, ctx, cfg)

			mustAllow(t, b, ctx) // never settled
			time.Sleep(cfg.ProbeTimeout + cfg.OpenFor)

			var admitted *circuit.Ticket
			require.PanicsWithValue(t, "observe boom", func() { admitted, _ = b.Allow(ctx) })
			require.Nil(t, admitted)
			require.Len(t, rec.observed(), 3)

			tk := mustAllow(t, b, ctx)
			require.NoError(t, nestedErr)
			got := rec.observed()
			require.Len(t, got, 4)
			require.Equal(t, circuit.Transition{From: circuit.Open, To: circuit.HalfOpen, Generation: 4}, got[3])

			require.True(t, tk.Done(nil))
			require.Equal(t, circuit.Closed, b.State())
			require.Len(t, rec.observed(), 5)
		})
	})

	t.Run("one goroutine delivers another's Transitions while the other's observer is blocked", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			cfg := testConfig()
			rec := &recorder{}
			entered := make(chan struct{})
			unblock := make(chan struct{})
			var once sync.Once
			observe := func(ctx context.Context, tr circuit.Transition) {
				rec.observe(ctx, tr)
				if tr.Generation == 1 {
					once.Do(func() { close(entered) })
					<-unblock
				}
			}
			b := newBreaker(t, cfg, circuit.Policy{Trip: tripOn(1), Probe: closeOnSuccess}, classifyByError, observe)
			ctx := t.Context()

			tripper := mustAllow(t, b, ctx)
			tripped := make(chan bool)
			go func() { tripped <- tripper.Done(errFailure) }()
			<-entered

			// The goroutine that tripped b is still delivering, so this call
			// queues its Transition for that goroutine and is admitted at
			// once.
			time.Sleep(cfg.OpenFor)
			other := context.WithValue(ctx, ctxKey{}, "other")
			tk := mustAllow(t, b, other)
			require.Len(t, rec.observed(), 1)

			close(unblock)
			require.True(t, <-tripped)
			require.Equal(t, []circuit.Transition{
				{From: circuit.Closed, To: circuit.Open, Generation: 1, Counts: circuit.Counts{Failures: 1, ConsecutiveFailures: 1}},
				{From: circuit.Open, To: circuit.HalfOpen, Generation: 2},
			}, rec.observed())
			require.Equal(t, "other", rec.contexts()[1].Value(ctxKey{}))
			require.False(t, rec.overlapped.Load())

			require.True(t, tk.Done(nil))
			require.Equal(t, circuit.Closed, b.State())
		})
	})

	t.Run("a slow observer does not use up a probe's timeout", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			cfg := testConfig()
			observe := func(_ context.Context, tr circuit.Transition) {
				if tr.To == circuit.HalfOpen {
					time.Sleep(2 * cfg.ProbeTimeout)
				}
			}
			pol := &policyRecorder{trip: tripOn(1), probe: reopenOnTimeout}
			b := newBreaker(t, cfg, pol.policy(), classifyByError, observe)
			ctx := t.Context()
			openThenWait(t, b, ctx, cfg)

			tk := mustAllow(t, b, ctx)
			require.True(t, tk.Done(nil))

			require.Equal(t, circuit.Closed, b.State())
			require.Equal(t, []circuit.Counts{{Successes: 1, ConsecutiveSuccesses: 1}}, pol.probeCounts())
		})
	})
}
