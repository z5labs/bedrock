// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package circuit_test

import (
	"context"
	"errors"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/z5labs/bedrock/circuit"
)

func TestBreaker_concurrency(t *testing.T) {
	t.Parallel()

	const (
		goroutines = 16
		operations = 2000
	)
	cfg := circuit.Config{Window: 20, Probes: 4, OpenFor: 200 * time.Microsecond, ProbeTimeout: 200 * time.Microsecond}

	var (
		policyBusy, observeBusy atomic.Int32
		violations              atomic.Int32
	)
	exclusive := func(busy *atomic.Int32) func() {
		if busy.Add(1) != 1 {
			violations.Add(1)
		}
		return func() { busy.Add(-1) }
	}
	checkCounts := func(c circuit.Counts) {
		inRange := c.Successes >= 0 && c.Failures >= 0 && c.Successes+c.Failures <= cfg.Window &&
			c.ConsecutiveSuccesses >= 0 && c.ConsecutiveFailures >= 0 &&
			(c.ConsecutiveSuccesses == 0 || c.ConsecutiveFailures == 0) &&
			c.TimedOut >= 0 && c.TimedOut <= operations*goroutines
		if !inRange {
			violations.Add(1)
		}
	}

	policy := circuit.Policy{
		Trip: func(c circuit.Counts) bool {
			defer exclusive(&policyBusy)()
			checkCounts(c)
			return c.ConsecutiveFailures >= 3
		},
		Probe: func(c circuit.Counts) circuit.State {
			defer exclusive(&policyBusy)()
			checkCounts(c)
			switch {
			case c.Failures > 0 || c.TimedOut > 0:
				return circuit.Open
			case c.Successes >= 2:
				return circuit.Closed
			default:
				return circuit.HalfOpen
			}
		},
	}

	var (
		mu          sync.Mutex
		transitions []circuit.Transition
	)
	observe := func(_ context.Context, tr circuit.Transition) {
		defer exclusive(&observeBusy)()
		checkCounts(tr.Counts)
		mu.Lock()
		defer mu.Unlock()
		transitions = append(transitions, tr)
	}

	b := newBreaker(t, cfg, policy, recommendedClassifier, observe)
	ctx := t.Context()
	errPanic := errors.New("panicked")

	// allow returns a Ticket, or nil after checking that the call was
	// rejected.
	allow := func(ctx context.Context) *circuit.Ticket {
		tk, err := b.Allow(ctx)
		if err != nil {
			if !errors.Is(err, circuit.ErrRejected) {
				t.Errorf("Allow returned %v, which is not a rejection", err)
			}
			return nil
		}
		return tk
	}

	var wg sync.WaitGroup
	for g := range goroutines {
		wg.Go(func() {
			r := rand.New(rand.NewPCG(uint64(g), 597))
			result := func() error {
				if r.IntN(3) == 0 {
					return errFailure
				}
				return nil
			}

			for range operations {
				switch op := r.IntN(7); op {
				case 0: // the idiom
					func() {
						tk := allow(ctx)
						if tk == nil {
							return
						}
						defer tk.Release()
						tk.Done(result())
					}()
				case 1: // a cancelled call
					func() {
						cctx, cancel := context.WithCancel(ctx)
						defer cancel()
						tk := allow(cctx)
						if tk == nil {
							return
						}
						defer tk.Release()
						cancel()
						tk.Done(cctx.Err())
					}()
				case 2: // a lost Ticket
					allow(ctx)
				case 3: // an early return
					func() {
						tk := allow(ctx)
						if tk == nil {
							return
						}
						defer tk.Release()
					}()
				case 4: // Done on another goroutine
					tk := allow(ctx)
					if tk == nil {
						continue
					}
					err := result()
					done := make(chan struct{})
					go func() {
						defer close(done)
						tk.Done(err)
					}()
					<-done
				case 5: // Done racing Release
					tk := allow(ctx)
					if tk == nil {
						continue
					}
					err := result()
					var race sync.WaitGroup
					race.Go(func() {
						if v := catchPanic(func() { tk.Done(err) }); v != nil && v != settledByRelease {
							t.Errorf("Done panicked with %v", v)
						}
					})
					race.Go(tk.Release)
					race.Wait()
				case 6: // a panic recovered higher up
					v := catchPanic(func() {
						tk := allow(ctx)
						if tk == nil {
							return
						}
						defer tk.Release()
						panic(errPanic)
					})
					if v != nil && v != errPanic {
						t.Errorf("recovered %v", v)
					}
				}
			}
		})
	}
	wg.Wait()

	require.Zero(t, violations.Load())

	// Afterwards the breaker still closes and admits calls.
	require.Eventually(t, func() bool {
		tk, err := b.Allow(ctx)
		if err != nil {
			return false
		}
		tk.Done(nil)
		return b.State() == circuit.Closed
	}, 10*time.Second, time.Millisecond)
	settle(t, b, ctx, nil)

	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, transitions)
	from := circuit.Closed
	for i, tr := range transitions {
		require.Equal(t, uint64(i+1), tr.Generation)
		require.Equal(t, from, tr.From)
		require.NotEqual(t, tr.From, tr.To)
		from = tr.To
	}
	require.Equal(t, circuit.Closed, from)
	require.Zero(t, violations.Load())
}
