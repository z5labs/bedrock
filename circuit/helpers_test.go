// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package circuit_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/z5labs/bedrock/circuit"
)

var (
	errFailure = errors.New("failure")
	errIgnored = errors.New("ignored")
)

// classifyByError counts nil as a Success, errIgnored as Ignored and every
// other error as a Failure.
func classifyByError(_ context.Context, err error) circuit.Outcome {
	switch {
	case err == nil:
		return circuit.Success
	case errors.Is(err, errIgnored):
		return circuit.Ignored
	default:
		return circuit.Failure
	}
}

// recommendedClassifier is the classifier discussion #595 recommends: it
// ignores a call only when its caller cancelled it.
func recommendedClassifier(ctx context.Context, err error) circuit.Outcome {
	if err == nil {
		return circuit.Success
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return circuit.Ignored
	}
	return circuit.Failure
}

func ignoreTransitions(context.Context, circuit.Transition) {}

// tripOn trips after n consecutive failures.
func tripOn(n int) func(circuit.Counts) bool {
	return func(c circuit.Counts) bool { return c.ConsecutiveFailures >= n }
}

// closeOnSuccess reopens on a failed probe, closes on a successful one, and
// otherwise keeps probing.
func closeOnSuccess(c circuit.Counts) circuit.State {
	switch {
	case c.Failures > 0:
		return circuit.Open
	case c.Successes > 0:
		return circuit.Closed
	default:
		return circuit.HalfOpen
	}
}

func neverTrip(circuit.Counts) bool { return false }

func stayHalfOpen(circuit.Counts) circuit.State { return circuit.HalfOpen }

// policyRecorder is a Policy that records the Counts each function is called
// with, and whose functions a test can swap.
type policyRecorder struct {
	mu     sync.Mutex
	trip   func(circuit.Counts) bool
	probe  func(circuit.Counts) circuit.State
	trips  []circuit.Counts
	probes []circuit.Counts
}

func (p *policyRecorder) policy() circuit.Policy {
	return circuit.Policy{
		Trip: func(c circuit.Counts) bool {
			p.mu.Lock()
			p.trips = append(p.trips, c)
			f := p.trip
			p.mu.Unlock()
			return f(c)
		},
		Probe: func(c circuit.Counts) circuit.State {
			p.mu.Lock()
			p.probes = append(p.probes, c)
			f := p.probe
			p.mu.Unlock()
			return f(c)
		},
	}
}

func (p *policyRecorder) setTrip(f func(circuit.Counts) bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.trip = f
}

func (p *policyRecorder) setProbe(f func(circuit.Counts) circuit.State) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.probe = f
}

func (p *policyRecorder) tripCounts() []circuit.Counts {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]circuit.Counts(nil), p.trips...)
}

func (p *policyRecorder) probeCounts() []circuit.Counts {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]circuit.Counts(nil), p.probes...)
}

// recorder records every Transition observed, with its context, and notes
// any call to observe that overlaps another.
type recorder struct {
	mu          sync.Mutex
	transitions []circuit.Transition
	ctxs        []context.Context

	inFlight   atomic.Int32
	overlapped atomic.Bool
}

func (r *recorder) observe(ctx context.Context, t circuit.Transition) {
	if r.inFlight.Add(1) != 1 {
		r.overlapped.Store(true)
	}
	defer r.inFlight.Add(-1)

	r.mu.Lock()
	defer r.mu.Unlock()
	r.transitions = append(r.transitions, t)
	r.ctxs = append(r.ctxs, ctx)
}

func (r *recorder) observed() []circuit.Transition {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]circuit.Transition(nil), r.transitions...)
}

func (r *recorder) contexts() []context.Context {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]context.Context(nil), r.ctxs...)
}

// switchClassifier is a classifier whose function a test can swap.
type switchClassifier struct {
	mu sync.Mutex
	fn func(context.Context, error) circuit.Outcome
}

func (s *switchClassifier) classify(ctx context.Context, err error) circuit.Outcome {
	s.mu.Lock()
	f := s.fn
	s.mu.Unlock()
	return f(ctx, err)
}

func (s *switchClassifier) set(f func(context.Context, error) circuit.Outcome) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fn = f
}

// testConfig is a valid Config with one probe slot.
func testConfig() circuit.Config {
	return circuit.Config{
		Window:       10,
		Probes:       1,
		OpenFor:      10 * time.Second,
		ProbeTimeout: 5 * time.Second,
	}
}

func newBreaker(t *testing.T, cfg circuit.Config, p circuit.Policy, classify func(context.Context, error) circuit.Outcome, observe func(context.Context, circuit.Transition)) *circuit.Breaker {
	t.Helper()

	b, err := circuit.New(cfg, p, classify, observe)
	require.NoError(t, err)
	require.NotNil(t, b)
	return b
}

func mustAllow(t *testing.T, b *circuit.Breaker, ctx context.Context) *circuit.Ticket {
	t.Helper()

	tk, err := b.Allow(ctx)
	require.NoError(t, err)
	require.NotNil(t, tk)
	return tk
}

// settle makes one call through b that returns err, and returns what Done
// reported.
func settle(t *testing.T, b *circuit.Breaker, ctx context.Context, err error) bool {
	t.Helper()

	tk := mustAllow(t, b, ctx)
	defer tk.Release()
	return tk.Done(err)
}

func requireRejected(t *testing.T, b *circuit.Breaker, ctx context.Context, want circuit.RejectedError) {
	t.Helper()

	tk, err := b.Allow(ctx)
	require.Nil(t, tk)
	require.ErrorIs(t, err, circuit.ErrRejected)

	got, ok := errors.AsType[*circuit.RejectedError](err)
	require.True(t, ok)
	require.Equal(t, want, *got)
}

// openThenWait trips b, which must trip on one failure, and waits out its
// open period, so that the next call to Allow makes b half-open. It must be
// called inside a synctest bubble.
func openThenWait(t *testing.T, b *circuit.Breaker, ctx context.Context, cfg circuit.Config) {
	t.Helper()

	settle(t, b, ctx, errFailure)
	require.Equal(t, circuit.Open, b.State())
	time.Sleep(cfg.OpenFor)
	require.Equal(t, circuit.HalfOpen, b.State())
}

// requireNextProbeCloses shows that the next call is admitted, so no probe
// slot leaked, and that its success closes b.
func requireNextProbeCloses(t *testing.T, b *circuit.Breaker, ctx context.Context) {
	t.Helper()

	tk := mustAllow(t, b, ctx)
	require.True(t, tk.Done(nil))
	require.Equal(t, circuit.Closed, b.State())
}

// catchPanic returns the value fn panicked with, or nil.
func catchPanic(fn func()) (v any) {
	defer func() { v = recover() }()
	fn()
	return nil
}
