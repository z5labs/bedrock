// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package circuit

import (
	"errors"
	"fmt"
	"time"
)

// State is the state of a [Breaker]. The zero State is not a state, so a
// [Policy.Probe] that returns one by mistake panics instead of choosing a
// state silently.
type State int

const (
	// Closed admits every call and counts its result.
	Closed State = iota + 1

	// Open rejects every call until Config.OpenFor has passed.
	Open

	// HalfOpen admits at most Config.Probes calls at once, the probes, and
	// counts their results.
	HalfOpen
)

// String returns "closed", "open" or "half-open", or "State(n)" for a
// value that is not a State.
func (s State) String() string {
	switch s {
	case Closed:
		return "closed"
	case Open:
		return "open"
	case HalfOpen:
		return "half-open"
	default:
		return fmt.Sprintf("State(%d)", int(s))
	}
}

// Outcome is how the result of one call counts. The zero Outcome is not an
// outcome, so a classifier that returns one by mistake panics instead of
// counting the result silently.
type Outcome int

const (
	// Success counts toward the dependency being healthy.
	Success Outcome = iota + 1

	// Failure counts toward the dependency being unhealthy.
	Failure

	// Ignored is evidence of nothing. It is not counted, it neither extends
	// nor ends a run of results, no Policy function is asked about it, and
	// it frees the probe slot it held.
	Ignored
)

// String returns "success", "failure" or "ignored", or "Outcome(n)" for a
// value that is not an Outcome.
func (o Outcome) String() string {
	switch o {
	case Success:
		return "success"
	case Failure:
		return "failure"
	case Ignored:
		return "ignored"
	default:
		return fmt.Sprintf("Outcome(%d)", int(o))
	}
}

// MaxCount is the largest Config.Window and Config.Probes that New accepts.
// Both are allocated when the Breaker is built, so the bound turns a typo in
// configuration into an error from New instead of a crash at startup.
const MaxCount = 1 << 16

// Config holds the settings of a Breaker that come from configuration. Every
// field is required: New rejects a zero or out-of-range value rather than
// replacing it with a default.
type Config struct {
	// Window is how many of the latest successes and failures in the
	// current state Counts.Successes and Counts.Failures cover, from 1 to
	// MaxCount.
	Window int

	// Probes is how many calls a half-open Breaker has in flight at once,
	// from 1 to MaxCount.
	Probes int

	// OpenFor is how long an open Breaker rejects every call before it
	// becomes half-open. It must be positive.
	OpenFor time.Duration

	// ProbeTimeout is how long a probe holds its slot. A probe that has
	// not been settled by then stops holding it, so that another probe can
	// be admitted, and is counted in Counts.TimedOut. The call itself is
	// not cancelled, and its result still counts if it arrives before the
	// Breaker changes state. It must be positive.
	ProbeTimeout time.Duration
}

// Counts are what a Policy decides from. They cover the current state of a
// Breaker only: every change of state starts them again from zero, and a
// result that arrives after the state its call was admitted in has ended is
// not counted.
type Counts struct {
	// Successes and Failures count the outcomes among the latest
	// Config.Window successes and failures, so their sum never exceeds
	// Config.Window.
	Successes int
	Failures  int

	// ConsecutiveSuccesses and ConsecutiveFailures are the length of the
	// current run of successes or of failures. They are not bounded by the
	// window, so a run longer than Config.Window still trips a Policy that
	// waits for it, and they stop at math.MaxInt rather than wrap. At most
	// one of them is non-zero. An Ignored result neither extends nor ends a
	// run.
	ConsecutiveSuccesses int
	ConsecutiveFailures  int

	// TimedOut counts the probes that held their slot for
	// Config.ProbeTimeout without being settled. It is zero while closed.
	// A probe that timed out still counts as a success or a failure if its
	// result arrives before the Breaker changes state.
	TimedOut int
}

// Policy decides when a Breaker changes state. Both functions are required.
// circuit ships no Policy: how many failures are too many is the caller's
// decision.
//
// They are called with the Breaker's lock held, with a copy of the Counts,
// on the goroutine of the Ticket.Done that counted a result, or, after a
// probe times out, of the call to Allow or Ticket.Done that found it timed
// out, which may be a call of another goroutine's. They must be fast,
// must decide from the Counts alone, must be safe for concurrent use, and
// must not call the Breaker. A Policy function that panics leaves the
// Breaker consistent: what was counted stays counted, and the state does
// not change.
type Policy struct {
	// Trip is called after every success or failure counted while closed.
	// It reports whether the Breaker opens.
	Trip func(Counts) bool

	// Probe is called while half-open after every success or failure of a
	// probe, and after every probe that times out. It returns Closed to
	// close the Breaker, Open to reopen it, or HalfOpen to keep probing.
	// Any other State panics.
	Probe func(Counts) State
}

// Transition is a change of state of a Breaker, as observe receives it.
type Transition struct {
	From, To State

	// Generation numbers the Transitions of a Breaker from 1. Each one's
	// is one more than the one delivered before it.
	Generation uint64

	// Counts are the final Counts of the state the Breaker left, including
	// the result or timeout that made the Policy move it. They are zero
	// for a move from Open to HalfOpen, which time makes.
	Counts Counts
}

// ErrRejected is matched, with errors.Is, by every error that Breaker.Allow
// returns for a call it does not admit.
var ErrRejected = errors.New("circuit: call rejected")

// RejectedError is the error of a call the Breaker does not admit. It
// matches ErrRejected.
type RejectedError struct {
	// State is Open, or HalfOpen when every probe slot is taken.
	State State

	// RetryAfter is how much longer the Breaker stays open, measured when
	// the call was rejected. It is zero when State is HalfOpen, since a slot
	// is freed as soon as a probe is settled.
	RetryAfter time.Duration
}

// Error implements the error interface. It renders as
// "circuit: call rejected: open for another 4.2s" or
// "circuit: call rejected: half-open, and every probe slot is taken".
func (e *RejectedError) Error() string {
	switch e.State {
	case Open:
		return fmt.Sprintf("%v: open for another %v", ErrRejected, e.RetryAfter)
	case HalfOpen:
		return fmt.Sprintf("%v: half-open, and every probe slot is taken", ErrRejected)
	default:
		return fmt.Sprintf("%v: %v", ErrRejected, e.State)
	}
}

// Unwrap returns ErrRejected.
func (e *RejectedError) Unwrap() error { return ErrRejected }

// validate returns an error naming every invalid field of c, or nil.
func (c Config) validate() error {
	var errs []error
	if c.Window < 1 || c.Window > MaxCount {
		errs = append(errs, fmt.Errorf("circuit: window must be between 1 and %d, got %d", MaxCount, c.Window))
	}
	if c.Probes < 1 || c.Probes > MaxCount {
		errs = append(errs, fmt.Errorf("circuit: probes must be between 1 and %d, got %d", MaxCount, c.Probes))
	}
	if c.OpenFor <= 0 {
		errs = append(errs, fmt.Errorf("circuit: open duration must be positive, got %v", c.OpenFor))
	}
	if c.ProbeTimeout <= 0 {
		errs = append(errs, fmt.Errorf("circuit: probe timeout must be positive, got %v", c.ProbeTimeout))
	}
	return errors.Join(errs...)
}
