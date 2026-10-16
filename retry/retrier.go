// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package retry

import (
	"context"
	"fmt"
	"iter"
	"sync/atomic"
	"time"
)

// Retrier retries the calls to one dependency. Build one with New at startup
// and share it between every goroutine that calls that dependency. Its
// fields are unexported, so New is the only way to build one, and every
// method of a Retrier not built by New panics.
type Retrier struct {
	maxAttempts int
	schedule    func(attempt int) time.Duration
	classify    func(context.Context, error) (Outcome, time.Duration)
	budget      *Budget
}

// New returns a Retrier that makes at most maxAttempts attempts per call,
// classifies the result of each with classify, waits between attempts as
// schedule says, and limits its retries with budget.
//
// classify returns the Outcome of an attempt's error and, for Retry, the
// least time to wait before the next attempt, such as a server's
// Retry-After, or 0 to wait only as long as schedule says. The delay before
// the next attempt is the larger of the two. For Retry the least time must
// not be negative, so one computed from a server's date is clamped at 0
// first; for Success and Stop it is ignored. schedule returns the delay
// after attempt n, from 1, as [Exponential] does. Both must be safe for
// concurrent use, and both are called by Attempt.Done, on the goroutine
// that calls it, with nothing locked.
//
// budget may be shared with other Retriers, and may be nil for a Retrier
// whose retries are limited only by maxAttempts, such as one that retries a
// connection at startup. A Retrier with a Budget is for calls on the
// request path, where retries of every caller add up.
//
// New panics if schedule or classify is nil, or if budget is not nil and
// was not built by NewBudget, which are programming errors. It returns an
// error if maxAttempts is less than 1, because maxAttempts comes from
// configuration and App.Run should exit non-zero at startup instead. A
// maxAttempts of math.MaxInt retries until a limit other than the attempt
// count stops the call.
func New(maxAttempts int, schedule func(attempt int) time.Duration, classify func(context.Context, error) (Outcome, time.Duration), budget *Budget) (*Retrier, error) {
	if schedule == nil {
		panic("retry: New called with a nil schedule")
	}
	if classify == nil {
		panic("retry: New called with a nil classify function")
	}
	if budget != nil && budget.max == 0 {
		panic("retry: New called with a Budget not built by NewBudget")
	}
	if maxAttempts < 1 {
		return nil, fmt.Errorf("retry: max attempts must be at least 1, got %d", maxAttempts)
	}
	return &Retrier{
		maxAttempts: maxAttempts,
		schedule:    schedule,
		classify:    classify,
		budget:      budget,
	}, nil
}

// Attempts returns the attempts of one call to r's dependency. It is the only
// way to make a call through r, and it sits inline in the caller's code:
//
//	var err error
//	for a := range r.Attempts(ctx) {
//		err = call(ctx)
//		a.Done(err)
//	}
//	return err
//
// The first attempt is yielded at once, even if ctx is already done. After
// an attempt for which Attempt.Done returned Again, the next one is yielded
// once the delay has passed, unless ctx ends first. The loop ends:
//
//   - after Attempt.Done returns any other Decision;
//   - when the loop body breaks or returns after Done, which skips the
//     wait after an Again, for a caller that must not retry a particular
//     call;
//   - when ctx ends during the wait. Then the last Decision was Again,
//     although the loop body did not break, and ctx.Err() is not nil.
//
// Each range over the sequence is a new call, which starts from attempt 1.
// ctx is the context of the call. The classifier receives it with every
// result, and its deadline ends the call when the next attempt would start
// too late.
//
// Attempt.Done may be called on another goroutine, as long as it returns
// before the loop body ends. A panic or runtime.Goexit in the loop body
// passes through unchanged, and ends the attempt it was in.
//
// Attempts panics if r was not built by New, or if ctx is nil.
func (r *Retrier) Attempts(ctx context.Context) iter.Seq[*Attempt] {
	r.mustBeBuilt("Attempts")
	if ctx == nil {
		panic("retry: Retrier.Attempts called with a nil context")
	}

	return func(yield func(*Attempt) bool) {
		// A loop body that panics or calls runtime.Goexit never returns to
		// the iterator, so end its attempt on the way out: a Done called on
		// it later panics instead of deciding for a call that is gone.
		// Nothing is recovered.
		var a *Attempt
		defer func() {
			if a != nil {
				a.state.CompareAndSwap(open, ended)
			}
		}()

		for n := 1; ; n++ {
			a = &Attempt{r: r, ctx: ctx, n: n}
			more := yield(a)
			d, delay := a.end()
			if !more || d != Again || !sleep(ctx, delay) {
				return
			}
		}
	}
}

// Attempt is one attempt of a call, yielded by Retrier.Attempts. Every
// attempt reports its result with Done, once, before the loop body ends,
// whether the body then continues, breaks or returns. An attempt that ends
// without Done panics, because a result that is never reported is lost:
// a lost Success never refills the Budget, and a lost failure is never
// counted. The exception is a loop body that panics or calls
// runtime.Goexit, which ends its attempt as it passes through.
type Attempt struct {
	r   *Retrier
	ctx context.Context
	n   int

	state    atomic.Uint32 // open, deciding, decided or ended
	decision Decision      // set before state becomes decided
	delay    time.Duration // likewise
}

// The states of an Attempt. Exactly one of Done, which moves an Attempt
// from open to deciding, and the end of the attempt, which moves it from
// open to ended, claims it.
const (
	open uint32 = iota
	deciding
	decided
	ended
)

// Number returns the number of a, from 1. A caller that must prepare a
// request again for each retry, such as one that rewinds an HTTP request
// body, does so when Number is greater than 1.
//
// Number panics if a was not yielded by Retrier.Attempts.
func (a *Attempt) Number() int {
	a.mustBeFromAttempts("Number")
	return a.n
}

// Done reports the result of a's call, and returns what happens next. It
// classifies err with the context passed to Retrier.Attempts, updates the
// Budget, and decides whether another attempt follows:
//
//   - Success: Succeeded, and the Budget gains BudgetConfig.TokenRatio;
//   - Stop: Stopped, and the Budget is unchanged;
//   - Retry, with the context done: Expired, and the Budget is unchanged,
//     because the caller giving up says nothing about the dependency;
//   - Retry otherwise: one token is taken from the Budget, and then, in
//     this order, Exhausted if a was the last attempt allowed, Throttled if
//     the Budget allows no retry, Expired if the context's deadline comes
//     before the delay would end, and Again otherwise.
//
// Call Done straight after the call, with its error, before any check of
// the error, so that every result is classified. A caller that must not
// retry this particular call, such as one whose request is not idempotent,
// breaks out of the loop after Done, which skips the wait.
//
// Again is what Done decides, not yet what happens: the wait that follows
// may be cut short by the context, and then no attempt follows. A result
// that the next attempt replaces, such as the body of a response that is
// retried, is therefore cleaned up when the next attempt starts, when
// Number is greater than 1, so that a call whose wait is cut short still
// ends with its last result.
//
// Done panics if it is called twice for one attempt, or after its attempt
// ended by a panic or runtime.Goexit, or if a was not yielded by
// Retrier.Attempts. It panics too if the classifier returns a value that is
// not an Outcome, or a negative delay with Retry, or if the schedule
// returns a negative delay. A panic from the classifier leaves the Budget
// as it was. One from the schedule comes after the Budget counted the
// Retry.
func (a *Attempt) Done(err error) Decision {
	a.mustBeFromAttempts("Done")
	if !a.state.CompareAndSwap(open, deciding) {
		if a.state.Load() == ended {
			panic(fmt.Sprintf("retry: Attempt.Done called after attempt %d ended", a.n))
		}
		panic(fmt.Sprintf("retry: Attempt.Done called twice for attempt %d", a.n))
	}

	d, delay := a.r.decide(a.ctx, a.n, err)
	a.decision, a.delay = d, delay
	a.state.Store(decided)
	return d
}

// end ends a once the loop body has returned to the iterator, and returns
// the Decision Done made and the delay before the next attempt.
func (a *Attempt) end() (Decision, time.Duration) {
	if a.state.CompareAndSwap(open, ended) {
		panic(fmt.Sprintf("retry: attempt %d ended without a call to Attempt.Done", a.n))
	}
	if a.state.Load() != decided {
		panic(fmt.Sprintf("retry: attempt %d ended before its call to Attempt.Done returned", a.n))
	}
	return a.decision, a.delay
}

// decide classifies err, the result of attempt n, and decides what follows.
func (r *Retrier) decide(ctx context.Context, n int, err error) (Decision, time.Duration) {
	o, after := r.classify(ctx, err)
	switch o {
	case Success:
		if r.budget != nil {
			r.budget.succeeded()
		}
		return Succeeded, 0
	case Stop:
		return Stopped, 0
	case Retry:
	default:
		panic(fmt.Sprintf("retry: the classifier returned %v, which is not an Outcome", o))
	}
	if after < 0 {
		panic(fmt.Sprintf("retry: the classifier returned a negative delay, %v, for a Retry", after))
	}

	// The caller gave up, or ran out of time, which says nothing about the
	// dependency, so it costs the Budget nothing.
	if ctx.Err() != nil {
		return Expired, 0
	}
	allowed := r.budget == nil || r.budget.failed()
	switch {
	case n >= r.maxAttempts:
		return Exhausted, 0
	case !allowed:
		return Throttled, 0
	}

	delay := r.schedule(n)
	if delay < 0 {
		panic(fmt.Sprintf("retry: the schedule returned a negative delay, %v, for attempt %d", delay, n))
	}
	delay = max(delay, after)
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) <= delay {
		return Expired, 0
	}
	return Again, delay
}

// sleep waits for d, or until ctx ends, and reports whether ctx is still
// live once the wait is over, so that a cancellation that has happened by
// then ends the call even if the timer fired too.
func sleep(ctx context.Context, d time.Duration) bool {
	if d > 0 {
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-ctx.Done():
		case <-t.C:
		}
	}
	return ctx.Err() == nil
}

// mustBeBuilt panics if r was not built by New.
func (r *Retrier) mustBeBuilt(method string) {
	if r == nil || r.classify == nil {
		panic("retry: Retrier." + method + " called on a Retrier not built by New")
	}
}

// mustBeFromAttempts panics if a was not yielded by Retrier.Attempts.
func (a *Attempt) mustBeFromAttempts(method string) {
	if a == nil || a.r == nil {
		panic("retry: Attempt." + method + " called on an Attempt not yielded by Retrier.Attempts")
	}
}
