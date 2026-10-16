// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package retry retries calls to a dependency.
//
// # Calls
//
// A call is retried inline, in the caller's own code, by ranging over the
// attempts that [Retrier.Attempts] yields. Each attempt makes the call and
// reports its result with [Attempt.Done]:
//
//	var err error
//	for a := range r.Attempts(ctx) {
//		err = call(ctx)
//		a.Done(err)
//	}
//	return err
//
// Done classifies the result and returns a [Decision]: [Again] when another
// attempt follows, or the reason none does. After Again, the next attempt
// is yielded once the delay has passed, unless ctx ends first. When the loop
// ends, err holds the result of the last attempt.
//
// Every attempt must reach Done before the loop body ends, whether the body
// then continues, breaks or returns, so that every result is classified. A
// Success that never reaches Done never refills the [Budget], and a
// failure that never reaches it is never counted, so an attempt that ends
// without Done panics. Call Done straight after the call, before any check
// of its error. Assign the call's results to variables declared before
// the loop: a := inside the loop body declares new ones, and the function
// returns the outer ones, which the loop never set.
//
// Because the loop is the caller's own code, the caller decides what only
// it knows about a call:
//
//   - a call that must not be retried, such as a request that is not
//     idempotent, breaks out of the loop after Done, whatever Done decided;
//   - a request that must be prepared again for a retry, such as one whose
//     body must be rewound, is prepared when [Attempt.Number] is greater
//     than 1, and the result of the attempt before it, such as the body of
//     a response, is cleaned up then too;
//   - a deadline for each attempt is set inside the loop with
//     context.WithTimeout.
//
// Again is what Done decides, not yet what happens. The context may end
// during the wait that follows, and then the loop ends with no further
// attempt: the last Decision was Again, although the loop body did not
// break, and ctx.Err() is not nil. A result is therefore cleaned up when
// the attempt that replaces it starts, not when Done returns Again, so a
// call whose wait is cut short still ends with its last result.
//
// A call can be retried only until its result is handed to its caller:
// once the caller reads a response body or a stream, the call is committed,
// and a failure after that is the caller's to handle.
//
// Each call belongs either at startup or on the request path:
//
//	| Call                                       | When                                         |
//	|--------------------------------------------|----------------------------------------------|
//	| Exponential, NewBudget, New                | Startup, inside App.Run, once per dependency |
//	| ranging over Attempts, with Done each time | Per outbound call                            |
//	| the classifier and the schedule            | Per Done, with nothing locked                |
//	| Budget.Tokens                              | Whenever, such as in a metrics callback      |
//
// One Retrier is shared by every goroutine that calls its dependency, so the
// classifier and the schedule must be safe for concurrent use.
//
// # Classifying results
//
// retry does not decide what can be retried. Every result passed to Done
// goes to the classify function passed to [New], together with the context
// passed to Attempts, and classify returns an [Outcome]:
//
//   - [Success]: the call succeeded, and no attempt follows;
//   - [Retry]: the call failed in a way another attempt may fix;
//   - [Stop]: the call failed in a way another attempt will not fix, or
//     must not try to.
//
// For Retry, classify also returns the least time to wait before the next
// attempt, such as a server's Retry-After, or 0. retry has no default
// classifier and treats no error specially, a nil error and the context
// errors included.
//
// The context passed to classify is the context of the whole call, so
// ctx.Err() tells the caller giving up from an attempt failing:
//
//   - ctx.Err() is not nil: the caller cancelled the call or its deadline
//     ran out, and another attempt cannot help. Done decides [Expired] for
//     a Retry here, and takes no token from the Budget for it, because the
//     caller giving up says nothing about the dependency; a classifier that
//     returns Stop says the same thing directly;
//   - ctx.Err() is nil, and err wraps context.DeadlineExceeded: a deadline
//     set for that one attempt ran out, which a retry may fix.
//
// A result that is not an error, such as an HTTP status code, reaches
// classify as an error value that the caller builds.
//
// # Composing with a circuit breaker
//
// A circuit breaker goes inside the retry loop, so that each attempt is a
// call the breaker admits or rejects. A breaker that rejects a call is
// telling its caller not to call, so retrying the rejection at once
// defeats the breaker. retry imports no breaker. The caller's classifier
// returns Stop for the breaker's rejection, such as an error that matches
// github.com/z5labs/bedrock/circuit's ErrRejected, and the attempt makes
// its call through the breaker in a function of its own, so that the
// breaker's deferred guard runs once per attempt:
//
//	for a := range r.Attempts(ctx) {
//		err = callThrough(ctx, b) // b.Allow, defer t.Release, the call, t.Done
//		a.Done(err)
//	}
//
// # Limiting retries
//
// Retries add load to a dependency when it can least take it. A limit on
// attempts per call bounds one call, but not every call together: when a
// dependency fails, every caller retries at once, and a service that
// retries at each of several layers multiplies its attempts. A [Budget]
// bounds every call that shares it, as gRPC's retry throttling does: each
// attempt classified Retry takes a token, each Success adds a fraction of
// one back, and retries stop while half the tokens or fewer are left.
// First attempts are never limited, so a call is never refused for lack of
// budget, only its retries.
//
// A Budget belongs to calls on the request path. A Retrier that retries a
// connection at startup, until a deadline or a signal stops it, has no
// Budget: pass nil to New, and math.MaxInt attempts, so that only the
// context ends the call, as [New] shows.
//
// # Schedules
//
// The delay before each retry comes from the schedule passed to New, a
// function from the number of the attempt that failed to a delay.
// [Exponential] builds the usual one, exponential backoff with full jitter.
// A schedule is a function of the attempt number alone, so one schedule is
// shared by every call.
//
// # Panics
//
// retry never recovers a panic. A panic in the loop body, or a call to
// runtime.Goexit, passes through the iterator unchanged and ends the
// attempt it was in, and retry holds nothing between attempts, so nothing
// is left behind. Misuse panics too, because it is a bug in the caller: an
// attempt that ends without Done, Done called twice or after its attempt
// ended, and a classifier or schedule that returns a value that is out of
// range.
//
// # Time
//
// retry reads time.Now and sets a timer for each wait, and nothing else: no
// clock is injected, and no goroutine runs in the background. A test runs a
// Retrier inside a testing/synctest bubble, where the waits take no real
// time. A cancellation timed to the very instant a wait ends may land before
// or after the next attempt starts, so a test cancels before or after that
// instant, not at it.
//
// # Out of scope
//
// retry does not:
//
//   - hedge calls, time out attempts, rate limit or bound the concurrency of
//     calls;
//   - share a Budget's tokens between processes: each process has its own
//     Budgets;
//   - keep a registry of named Retriers: the caller holds each Retrier it
//     builds;
//   - replay request bodies or parse Retry-After or gRPC pushback, which
//     belong to the framework that wraps a transport;
//   - ship classifiers.
package retry
