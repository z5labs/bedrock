// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package retry

import "fmt"

// Outcome is how a [Retrier] treats the result of one attempt, as the
// classifier passed to [New] decides it. The zero Outcome is not an outcome,
// so a classifier that returns one by mistake panics instead of deciding
// silently.
type Outcome int

const (
	// Success ends the call: no attempt follows. It adds
	// BudgetConfig.TokenRatio tokens to the Budget.
	Success Outcome = iota + 1

	// Retry is a failure that another attempt may fix, such as a dependency
	// that is briefly unavailable. It takes one token from the Budget, and
	// another attempt follows if the attempt limit, the Budget and the
	// context all allow one.
	Retry

	// Stop is a failure that another attempt will not fix, or must not
	// try to, such as a call rejected by an open circuit breaker. It ends
	// the call, and leaves the Budget as it is.
	Stop
)

// String returns "success", "retry" or "stop", or "Outcome(n)" for a value
// that is not an Outcome.
func (o Outcome) String() string {
	switch o {
	case Success:
		return "success"
	case Retry:
		return "retry"
	case Stop:
		return "stop"
	default:
		return fmt.Sprintf("Outcome(%d)", int(o))
	}
}

// Decision is what [Attempt.Done] decides after an attempt: [Again], or the
// reason no attempt follows. Done never returns the zero Decision.
type Decision int

const (
	// Again means another attempt follows once the delay has passed, unless
	// the context ends first.
	Again Decision = iota + 1

	// Succeeded means the classifier returned Success.
	Succeeded

	// Stopped means the classifier returned Stop.
	Stopped

	// Exhausted means the classifier returned Retry for the last attempt the
	// Retrier allows.
	Exhausted

	// Throttled means the classifier returned Retry, and the Budget allows
	// no retry.
	Throttled

	// Expired means the classifier returned Retry, and the context is done,
	// or its deadline comes before the next attempt would start.
	Expired
)

// String returns "again", "succeeded", "stopped", "exhausted", "throttled"
// or "expired", or "Decision(n)" for a value that is not a Decision.
func (d Decision) String() string {
	switch d {
	case Again:
		return "again"
	case Succeeded:
		return "succeeded"
	case Stopped:
		return "stopped"
	case Exhausted:
		return "exhausted"
	case Throttled:
		return "throttled"
	case Expired:
		return "expired"
	default:
		return fmt.Sprintf("Decision(%d)", int(d))
	}
}
