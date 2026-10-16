// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package circuit

import (
	"context"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

// Breaker is a circuit breaker for one dependency. Build one with New at
// startup and share it between every goroutine that calls that dependency.
// Its fields are unexported, so New is the only way to build one, and every
// method of a Breaker not built by New panics.
type Breaker struct {
	cfg      Config
	policy   Policy
	classify func(context.Context, error) Outcome
	observe  func(context.Context, Transition)

	mu sync.Mutex

	// state is the state of the Breaker as of the last change it made.
	// Every change increments generation and sets since to the moment it
	// fell due, so an Open state ends at since+cfg.OpenFor.
	state      State
	generation uint64
	since      time.Time

	// counts are the Counts of the current state. ring holds the latest
	// ringLen successes and failures, true for a failure, oldest at
	// ringNext once it is full.
	counts   Counts
	ring     []bool
	ringLen  int
	ringNext int

	// probes holds one slot per Config.Probes. A slot in use is linked,
	// from head to tail, in the order its probe was admitted, which is
	// the order the probes time out in, and a free slot is on free. seq
	// numbers the probes admitted, from 1, so a Ticket recognises its
	// slot, and a slot whose seq is 0 is free.
	probes     []probe
	free       []int
	head, tail int
	seq        uint64

	// queue holds the Transitions made and not yet delivered, oldest
	// first. delivering is set while a goroutine delivers them.
	queue      []pending
	delivering bool
}

// probe is a slot of a half-open Breaker.
type probe struct {
	seq        uint64
	admittedAt time.Time
	prev, next int
}

// pending is a Transition not yet delivered, with the context of the call
// that made it.
type pending struct {
	ctx context.Context
	t   Transition
}

// New returns a closed Breaker with the settings cfg, that classifies every
// result with classify, changes state as p decides, and reports every
// Transition to observe.
//
// classify and observe must be safe for concurrent use. classify is called
// by Ticket.Done, on the goroutine that calls it, with the Breaker unlocked.
// observe is called with the Breaker unlocked, one Transition at a time and
// in order, so it may call the Breaker's methods.
//
// New panics if p.Trip, p.Probe, classify or observe is nil, which is a
// programming error. It returns an error naming every invalid field of cfg,
// because cfg comes from configuration and App.Run should exit non-zero at
// startup instead.
func New(cfg Config, p Policy, classify func(context.Context, error) Outcome, observe func(context.Context, Transition)) (*Breaker, error) {
	if p.Trip == nil {
		panic("circuit: New called with a nil Policy.Trip")
	}
	if p.Probe == nil {
		panic("circuit: New called with a nil Policy.Probe")
	}
	if classify == nil {
		panic("circuit: New called with a nil classify function")
	}
	if observe == nil {
		panic("circuit: New called with a nil observe function")
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	b := &Breaker{
		cfg:      cfg,
		policy:   p,
		classify: classify,
		observe:  observe,
		state:    Closed,
		ring:     make([]bool, cfg.Window),
		probes:   make([]probe, cfg.Probes),
		free:     make([]int, cfg.Probes),
		head:     -1,
		tail:     -1,
	}
	for i := range b.free {
		b.free[i] = cfg.Probes - 1 - i
	}
	return b, nil
}

// Allow admits a call and returns the [*Ticket] that settles it, or returns
// a nil Ticket and a [*RejectedError] if b does not admit the call. It is the
// only way to make a call through b, and it sits inline in the caller's
// code:
//
//	t, err := b.Allow(ctx)
//	if err != nil {
//		return err // a *circuit.RejectedError
//	}
//	defer t.Release()
//
//	err = call(ctx)
//	t.Done(err)
//
// ctx is the context of the call. The classifier receives it when the
// Ticket is settled with Ticket.Done, and observe receives it for any
// Transition that Allow makes. Allow delivers the Transitions already
// queued before it admits the call, unless another goroutine is delivering
// them, so observe never runs on this goroutine while the call holds a
// probe slot.
//
// Allow panics if ctx is nil, or if b was not built by New.
func (b *Breaker) Allow(ctx context.Context) (*Ticket, error) {
	b.mustBeBuilt("Allow")
	if ctx == nil {
		panic("circuit: Breaker.Allow called with a nil context")
	}

	for {
		t, queued, err := b.tryAdmit(ctx)
		if !queued {
			return t, err
		}
		b.deliver()
	}
}

// State returns the state of b as time has left it. Once b has been open for
// Config.OpenFor, State returns HalfOpen, although b makes that Transition,
// and calls observe, only when the next call to Allow or Ticket.Done
// arrives. State does not ask the Policy about a probe that has timed out:
// that call does, and may find b open again. State changes nothing and calls
// no function of the caller's, so a metrics callback may call it.
//
// State panics if b was not built by New.
func (b *Breaker) State() State {
	b.mustBeBuilt("State")

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.state == Open && !time.Now().Before(b.since.Add(b.cfg.OpenFor)) {
		return HalfOpen
	}
	return b.state
}

// Ticket is one call admitted by Breaker.Allow. It is settled once, by the
// first of Done, with the result of the call, and Release, without one. A
// Ticket may be settled from any goroutine.
//
// Defer Release straight after Allow, as cancel is deferred after
// context.WithCancel or Rollback after sql.DB.BeginTx. The deferred Release
// frees the slot the call held if the code before Done panics, calls
// runtime.Goexit or returns early, and does nothing once Done has settled
// the Ticket. Done after Release panics, because a result that arrives
// after the guard settled the call is a bug.
//
// A Ticket that is never settled costs nothing while the Breaker is closed.
// While it is half-open, the probe holds its slot for at most
// Config.ProbeTimeout.
type Ticket struct {
	b    *Breaker
	ctx  context.Context
	gen  uint64 // the generation the call was admitted in
	seq  uint64 // the probe's seq, or 0 for a call admitted while closed
	slot int    // the probe's slot, if seq is not 0

	settled atomic.Uint32 // unsettled, settledByDone or settledByRelease
}

// How a Ticket was settled.
const (
	unsettled uint32 = iota
	settledByDone
	settledByRelease
)

// Done settles t with the result of its call: it classifies err with the
// context passed to Allow, and counts it. It reports whether the result
// reached the state the call was admitted in: false means the Breaker had
// changed state since, and the result was discarded.
//
// Call Done straight after the call, with its error, and not from a
// deferred function: defer t.Done(err) evaluates err when it is deferred,
// and a deferred Done classifies a result even while a panic unwinds.
//
// A classifier that panics or returns an invalid Outcome frees t's slot,
// as Release does, before the panic continues. Done panics if t is already
// settled, whether by Done or by Release, or if t was not returned by
// Breaker.Allow, so code that may reach Done from several places, such as
// both Read and Close of a response body, guards it with a sync.Once.
func (t *Ticket) Done(err error) bool {
	t.mustBeFromAllow("Done")
	if !t.settled.CompareAndSwap(unsettled, settledByDone) {
		by := "Done"
		if t.settled.Load() == settledByRelease {
			by = "Release"
		}
		panic("circuit: Ticket.Done called on a Ticket already settled by " + by)
	}

	b := t.b
	classified := false
	defer func() {
		if !classified {
			b.abandon(t) // the classifier panicked
		}
	}()
	o := b.classifyChecked(t.ctx, err)
	classified = true
	counted, deliver := b.settle(t, o)
	if deliver {
		b.deliver()
	}
	return counted
}

// Release settles t without a result if it is not settled yet, and does
// nothing if it is. A call settled by Release is not counted, and frees the
// probe slot it held. Release calls no function of the caller's: not the
// classifier, the Policy or observe. It is therefore safe while a panic
// unwinds, and is the guard deferred straight after Allow:
//
//	defer t.Release()
//
// The one exception is a result that is settled after the function
// returns, such as by a response body that the caller reads later. A
// deferred Release would settle the call before the body is read, so the
// guard is a flag, set once the Ticket is handed off:
//
//	handedOff := false
//	defer func() {
//		if !handedOff {
//			t.Release() // next panicked, or called runtime.Goexit
//		}
//	}()
//	resp, err := next.RoundTrip(req)
//	if err != nil {
//		t.Done(err)
//		return nil, err
//	}
//	resp.Body = &body{ReadCloser: resp.Body, ticket: t} // settles t when read
//	handedOff = true
//	return resp, nil
//
// Release panics if t was not returned by Breaker.Allow.
func (t *Ticket) Release() {
	t.mustBeFromAllow("Release")
	if t.settled.CompareAndSwap(unsettled, settledByRelease) {
		t.b.abandon(t)
	}
}

// mustBeFromAllow panics if t was not returned by Breaker.Allow.
func (t *Ticket) mustBeFromAllow(method string) {
	if t == nil || t.b == nil {
		panic("circuit: Ticket." + method + " called on a Ticket not returned by Breaker.Allow")
	}
}

func (b *Breaker) mustBeBuilt(method string) {
	if b == nil || b.classify == nil {
		panic("circuit: Breaker." + method + " called on a Breaker not built by New")
	}
}

// classifyChecked classifies err, with b unlocked, and panics if the
// classifier returns a value that is not an Outcome.
func (b *Breaker) classifyChecked(ctx context.Context, err error) Outcome {
	o := b.classify(ctx, err)
	if o < Success || o > Ignored {
		panic(fmt.Sprintf("circuit: the classifier returned %v, which is not an Outcome", o))
	}
	return o
}

// tryAdmit admits a call, or returns the *RejectedError of one it does not
// admit, or claims the delivery of the Transitions queued when the call
// arrives, which this goroutine then makes before it tries again.
func (b *Breaker) tryAdmit(ctx context.Context) (*Ticket, bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	b.catchUp(ctx, now)
	if b.claimDelivery() {
		return nil, true, nil
	}
	switch b.state {
	case Closed:
		return &Ticket{b: b, ctx: ctx, gen: b.generation}, false, nil
	case Open:
		return nil, false, &RejectedError{State: Open, RetryAfter: b.since.Add(b.cfg.OpenFor).Sub(now)}
	}
	if len(b.free) == 0 {
		return nil, false, &RejectedError{State: HalfOpen}
	}

	i := b.free[len(b.free)-1]
	b.free = b.free[:len(b.free)-1]
	b.seq++
	b.probes[i] = probe{seq: b.seq, admittedAt: now, prev: b.tail, next: -1}
	if b.tail >= 0 {
		b.probes[b.tail].next = i
	} else {
		b.head = i
	}
	b.tail = i
	return &Ticket{b: b, ctx: ctx, gen: b.generation, seq: b.seq, slot: i}, false, nil
}

// settle counts the outcome o of the call t, and reports whether it reached
// the state t was admitted in, and whether it claimed the delivery of the
// Transitions queued, which this goroutine then makes.
//
// The call's slot is freed before any Policy function is called, so a
// Policy function that panics does not leak it.
func (b *Breaker) settle(t *Ticket, o Outcome) (counted, deliver bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	b.release(t, now)
	b.catchUp(t.ctx, now)
	counted = t.gen == b.generation
	if counted && o != Ignored {
		b.count(o == Failure)
		if b.state == Closed {
			if b.policy.Trip(b.counts) {
				b.transition(t.ctx, Open, now)
			}
		} else {
			b.probed(t.ctx, now)
		}
	}
	return counted, b.claimDelivery()
}

// abandon settles the call t without a result, and calls no function of
// the caller's.
func (b *Breaker) abandon(t *Ticket) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.release(t, time.Now())
}

// release frees the slot of the probe t, if t is a probe that still holds
// one. A probe past its timeout keeps its slot until catchUp times it out,
// so that whether it is counted in Counts.TimedOut does not depend on
// whether a call noticed the timeout before it was settled.
func (b *Breaker) release(t *Ticket, now time.Time) {
	if t.seq == 0 || b.probes[t.slot].seq != t.seq {
		return
	}
	if now.Before(b.probes[t.slot].admittedAt.Add(b.cfg.ProbeTimeout)) {
		b.unlink(t.slot)
	}
}

// catchUp makes every change that time has made due by now, oldest first,
// each as of the moment it fell due.
func (b *Breaker) catchUp(ctx context.Context, now time.Time) {
	for {
		switch {
		case b.state == Open:
			due := b.since.Add(b.cfg.OpenFor)
			if now.Before(due) {
				return
			}
			b.transition(ctx, HalfOpen, due)
		case b.state == HalfOpen && b.head >= 0:
			due := b.probes[b.head].admittedAt.Add(b.cfg.ProbeTimeout)
			if now.Before(due) {
				return
			}
			b.unlink(b.head)
			b.counts.TimedOut = increment(b.counts.TimedOut)
			b.probed(ctx, due)
		default:
			return
		}
	}
}

// count adds a success or a failure to the Counts of the current state.
func (b *Breaker) count(failure bool) {
	c := &b.counts
	if b.ringLen == len(b.ring) {
		if b.ring[b.ringNext] {
			c.Failures--
		} else {
			c.Successes--
		}
	} else {
		b.ringLen++
	}
	b.ring[b.ringNext] = failure
	b.ringNext = (b.ringNext + 1) % len(b.ring)

	if failure {
		c.Failures++
		c.ConsecutiveFailures = increment(c.ConsecutiveFailures)
		c.ConsecutiveSuccesses = 0
	} else {
		c.Successes++
		c.ConsecutiveSuccesses = increment(c.ConsecutiveSuccesses)
		c.ConsecutiveFailures = 0
	}
}

// increment returns n+1, or math.MaxInt rather than wrap.
func increment(n int) int {
	return min(n, math.MaxInt-1) + 1
}

// probed asks Policy.Probe what a half-open Breaker does next, and does it
// as of at.
func (b *Breaker) probed(ctx context.Context, at time.Time) {
	switch s := b.policy.Probe(b.counts); s {
	case HalfOpen:
	case Closed, Open:
		b.transition(ctx, s, at)
	default:
		panic(fmt.Sprintf("circuit: Policy.Probe returned %v, which is not a State", s))
	}
}

// transition moves b to the state to as of at, frees every probe slot,
// starts the Counts again from zero, and queues the Transition.
func (b *Breaker) transition(ctx context.Context, to State, at time.Time) {
	b.generation++
	b.queue = append(b.queue, pending{
		ctx: ctx,
		t:   Transition{From: b.state, To: to, Generation: b.generation, Counts: b.counts},
	})
	for b.head >= 0 {
		b.unlink(b.head)
	}
	b.state = to
	b.since = at
	b.counts = Counts{}
	b.ringLen, b.ringNext = 0, 0
}

// unlink frees the probe slot i.
func (b *Breaker) unlink(i int) {
	p := &b.probes[i]
	if p.prev >= 0 {
		b.probes[p.prev].next = p.next
	} else {
		b.head = p.next
	}
	if p.next >= 0 {
		b.probes[p.next].prev = p.prev
	} else {
		b.tail = p.prev
	}
	*p = probe{}
	b.free = append(b.free, i)
}

// claimDelivery makes this goroutine the one that delivers the queued
// Transitions, and reports whether it did. It does not when nothing is
// queued, or when another goroutine is delivering, which then delivers
// what is queued too. b must be locked.
func (b *Breaker) claimDelivery() bool {
	if len(b.queue) == 0 || b.delivering {
		return false
	}
	b.delivering = true
	return true
}

// deliver delivers the queued Transitions to observe, one at a time and
// with b unlocked. The goroutine that calls it has claimed the delivery
// with claimDelivery, so nothing else delivers until it returns.
func (b *Breaker) deliver() {
	b.mu.Lock()
	for len(b.queue) > 0 {
		p := b.queue[0]
		b.queue[0] = pending{}
		b.queue = b.queue[1:]
		b.mu.Unlock()
		b.deliverOne(p)
		b.mu.Lock()
	}
	b.delivering = false
	b.mu.Unlock()
}

// deliverOne calls observe with p. If observe panics, the panic continues,
// and the next call to Allow or Ticket.Done delivers what is queued.
func (b *Breaker) deliverOne(p pending) {
	observed := false
	defer func() {
		if !observed {
			b.mu.Lock()
			b.delivering = false
			b.mu.Unlock()
		}
	}()
	b.observe(p.ctx, p.t)
	observed = true
}
