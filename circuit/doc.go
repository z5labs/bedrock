// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package circuit models a circuit breaker.
//
// # Background
//
// A circuit breaker stops a process from calling a dependency that keeps
// failing, and lets a few calls through now and then to learn when it has
// recovered, as described in Michael Nygard's Release It! (2007) and in
// https://learn.microsoft.com/en-us/azure/architecture/patterns/circuit-breaker.
// A [Breaker] stands in front of one dependency, such as one host or one
// gRPC service, and is in one of three states:
//
//   - [Closed]: every call is admitted, and its result is counted. After
//     each success or failure, [Policy.Trip] decides whether to open;
//   - [Open]: every call is rejected with a [*RejectedError], until
//     [Config.OpenFor] has passed. The first call after that makes the
//     Breaker half-open;
//   - [HalfOpen]: at most [Config.Probes] calls, the probes, are in flight
//     at once, and the rest are rejected. After each probe's success or
//     failure, and after each probe that times out, [Policy.Probe] decides
//     whether to close, to reopen or to keep probing.
//
// circuit models only that state machine. It does no I/O, starts no
// goroutine and sets no timer. Wrapping an http.RoundTripper or a gRPC
// connection, exporting metrics and reading [Config] from the environment
// belong to the framework built on it.
//
// # Calls
//
// A call goes through a Breaker inline, in the caller's own code, rather
// than as a function the Breaker runs. [Breaker.Allow] admits the call and
// returns a [*Ticket], or rejects it with a [*RejectedError], which matches
// [ErrRejected] and says how much longer the Breaker stays open. The Ticket
// is settled once, by the first of [Ticket.Done], with the result, and
// [Ticket.Release], without one. Release does nothing once the Ticket is
// settled, so a Release deferred straight after Allow guards the call, as
// cancel does after context.WithCancel:
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
// The deferred Release frees the slot the call held if the code before
// Done panics, calls runtime.Goexit or returns early. Done after Release
// panics, because a result that arrives after the guard settled the call
// is a bug.
//
// The classifier sees only what reaches Done: a call settled by Release
// counts nothing. Each result path must therefore reach Done, or its
// failures are lost, and a Breaker whose failures are lost never opens.
// Settling with t.Done(err) straight after the call, before any check of
// err, is the pattern that ensures it.
//
// The one exception to the deferred Release is a result that is settled
// after the function returns, such as by a response body that the caller
// reads later. There a deferred Release would settle the call before the
// body is read, so the guard is a flag set once the Ticket is handed off,
// as [Ticket.Release] shows.
//
// Each call belongs either at startup or on the request path:
//
//	| Call                                       | When                                            |
//	|--------------------------------------------|-------------------------------------------------|
//	| Reading Config, building the Policy        | Startup, inside App.Run                         |
//	| New                                        | Startup, once per dependency                    |
//	| Allow, then Done, guarded by Release       | Per outbound call                               |
//	| the classifier                             | Per Done, with the Breaker unlocked             |
//	| Policy functions                           | Per success, failure or timeout, Breaker locked |
//	| observe                                    | Per change of state, with the Breaker unlocked  |
//	| State                                      | Whenever, such as in a metrics callback         |
//
// One Breaker is shared by every goroutine that calls its dependency, so
// the classifier, the Policy functions and observe must be safe for
// concurrent use.
//
// # Classifying results
//
// circuit does not decide what a failure is. Every result passed to
// Ticket.Done goes to the classify function passed to [New], together with
// the context passed to Allow, and classify returns a [Success], a [Failure]
// or [Ignored]. An Ignored result is evidence of nothing: it is not counted,
// it neither extends nor ends a run of results, and it frees the probe slot
// it held. circuit has no default classifier and treats no error specially,
// a nil error and the context errors included.
//
// The context lets classify tell the caller giving up from the dependency
// failing. ctx.Err() tells three cases apart:
//
//   - context.Canceled: the caller gave up, which says nothing about the
//     dependency;
//   - context.DeadlineExceeded: the caller's deadline ran out while it
//     waited for the dependency. http.Client.Timeout and a gRPC per-call
//     deadline both end a call this way, because both are deadlines on the
//     context the request is made with, which is the context the Breaker
//     sees. A dependency that hangs ends every call this way, so a
//     classifier that ignores this case never opens the Breaker. The
//     deadline may also be one inherited from an inbound request;
//   - nil, with an err that wraps context.DeadlineExceeded: a deadline set
//     inside the call ran out.
//
// Which of these count against the dependency is the caller's decision.
// This classifier counts every deadline and ignores only the caller giving
// up:
//
//	func classify(ctx context.Context, err error) circuit.Outcome {
//		switch {
//		case err == nil:
//			return circuit.Success
//		case errors.Is(ctx.Err(), context.Canceled):
//			return circuit.Ignored // the caller gave up
//		default:
//			return circuit.Failure // including every deadline
//		}
//	}
//
// A result that is not an error, such as an HTTP status code, reaches
// classify as an error value that the caller builds, such as a
// StatusError{Code: 503} passed to Ticket.Done.
//
// # Counts and policy
//
// circuit decides which counts a Breaker keeps, and the caller decides what
// they mean. [Counts] holds the successes and failures among the latest
// [Config.Window] results, the length of the current run of successes or of
// failures, which the window does not bound, and the probes that timed out.
// Every change of state starts the Counts again from zero.
//
// A [Policy] decides from the Counts alone, so it is a pure function of a
// value, and can be tested without a Breaker:
//
//	circuit.Policy{
//		// Open after 5 failures in a row, or when half of the last 20 or
//		// more results failed.
//		Trip: func(c circuit.Counts) bool {
//			n := c.Successes + c.Failures
//			return c.ConsecutiveFailures >= 5 || (n >= 20 && 2*c.Failures >= n)
//		},
//		// Reopen on any failed probe, close after 3 successful ones, and
//		// keep probing otherwise, including after a probe times out.
//		Probe: func(c circuit.Counts) circuit.State {
//			switch {
//			case c.Failures > 0:
//				return circuit.Open
//			case c.ConsecutiveSuccesses >= 3:
//				return circuit.Closed
//			default:
//				return circuit.HalfOpen
//			}
//		},
//	}
//
// The window counts results, not time. At low traffic the latest Window
// results can reach back a long way, so a policy that must react only to
// recent results relies on the consecutive counts. circuit ships no
// Policy.
//
// Policy functions run with the Breaker locked, so they must be fast and
// must not call the Breaker.
//
// # Late results
//
// A result belongs to the state its call was admitted in. When it arrives
// after the Breaker has changed state, for example a slow call admitted
// while closed that finishes after the Breaker opened, it is discarded:
// [Ticket.Done] reports false, and the Counts of the newer state are
// untouched. A late result never closes, reopens or trips a Breaker in a
// state it was not admitted in.
//
// # Panics and lost results
//
// circuit never recovers a panic. A panic is a bug in the caller, not a sign
// that the dependency is unhealthy, so when the code between Allow and Done
// panics or calls runtime.Goexit, the deferred Release frees the slot the
// call held, counts nothing, and lets the panic continue. Release calls no
// function of the caller's, so it cannot panic again while a panic unwinds.
// A classifier that panics or returns an invalid [Outcome] frees the slot
// too, inside Done, before the panic continues. A probe that panics, and is
// recovered higher up as net/http's server does, therefore does not keep
// the Breaker from probing again.
//
// No guard frees the slot of a call that never returns, whose deferred
// Release never runs, nor of a Ticket handed off to something that never
// settles it, such as a response body that is never read or closed. A probe
// therefore holds its slot for at most [Config.ProbeTimeout]. A probe still
// in flight by then stops holding it, so that another probe can be admitted,
// and is counted in [Counts.TimedOut], and Policy.Probe decides what that
// means: a Policy that reopens when a probe times out treats a hung
// dependency as failing, and one that keeps probing waits for results. The
// timeout does not cancel the call, and its result still counts if it
// arrives before the Breaker changes state, so a dependency slower than
// ProbeTimeout can still close the Breaker. While closed, a Ticket that is
// never settled costs nothing.
//
// # Observing transitions
//
// The observe function passed to New receives every [Transition], one at a
// time and in order: each one's Generation is one more than the last one's,
// and its From is the last one's To. observe is never called with the
// Breaker locked, and never concurrently with itself, so it may call the
// Breaker's methods, and an observer that sets a gauge needs no lock of its
// own. observe receives the context of the call that made the change,
// which may already be done.
//
// A Transition is delivered before the call that made it returns, unless
// another goroutine is already delivering, in which case that goroutine
// delivers it. If observe panics, the panic continues, and the Transitions
// still queued are delivered by the next call to Allow or Ticket.Done,
// which are the calls that make Transitions. A call to Allow delivers what
// is queued before it is admitted, so observe never runs while that call
// holds a probe slot.
//
// Results and rejections are not observed, because the caller already sees
// them: classify receives every result, and Allow returns every rejection.
// A framework counts them by wrapping the classifier and by checking for
// ErrRejected. circuit ships no observe function.
//
// # Time
//
// circuit reads time.Now and nothing else: no clock is injected, and no
// timer or goroutine runs in the background. Every period is measured with
// the monotonic clock. A change that time makes due, becoming half-open or
// a probe timing out, is made by the next call to Allow or Ticket.Done, as
// of the moment it fell due, so the Breaker behaves the same whichever
// call makes it. [Breaker.State] reports the state as time has left it
// without making the change, so a metrics gauge that reads it shows
// half-open as soon as the open period has passed, with no traffic.
//
// A test drives a Breaker through its states inside a testing/synctest
// bubble, advancing time with time.Sleep.
//
// # Out of scope
//
// circuit does not:
//
//   - retry, back off, time out, rate limit or bound the concurrency of
//     calls;
//   - share a Breaker's state between processes: each process has its own
//     Breakers;
//   - keep a registry of named Breakers: the caller holds each Breaker it
//     builds;
//   - ship transports, policies, classifiers or observers.
package circuit
