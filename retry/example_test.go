// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package retry_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/z5labs/bedrock/retry"
)

var errNotFound = errors.New("not found")

// classify retries every failure but two: the caller giving up, and a
// result that another attempt would only repeat.
func classify(ctx context.Context, err error) (retry.Outcome, time.Duration) {
	switch {
	case err == nil:
		return retry.Success, 0
	case ctx.Err() != nil:
		return retry.Stop, 0 // the caller cancelled the call, or its deadline ran out
	case errors.Is(err, errNotFound):
		return retry.Stop, 0
	default:
		return retry.Retry, 0
	}
}

// A call that fails twice and then succeeds. Each attempt reports its result
// with Done, and the loop ends when Done decides that no attempt follows.
func Example() {
	ctx := context.Background()

	// At startup, from configuration. The delays are short so that the
	// example runs quickly.
	schedule, err := retry.Exponential(time.Millisecond, 10*time.Millisecond)
	if err != nil {
		fmt.Println(err)
		return
	}
	budget, err := retry.NewBudget(retry.BudgetConfig{MaxTokens: 10, TokenRatio: 0.1})
	if err != nil {
		fmt.Println(err)
		return
	}
	r, err := retry.New(4, schedule, classify, budget)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Per outbound call.
	unavailable := errors.New("payments: unavailable")
	results := []error{unavailable, unavailable, nil}

	var callErr error
	for a := range r.Attempts(ctx) {
		callErr = results[a.Number()-1]
		fmt.Printf("attempt %d: %v: %v\n", a.Number(), callErr, a.Done(callErr))
	}
	fmt.Println("call:", callErr)
	fmt.Println("tokens:", budget.Tokens())
	// Output:
	// attempt 1: payments: unavailable: again
	// attempt 2: payments: unavailable: again
	// attempt 3: <nil>: succeeded
	// call: <nil>
	// tokens: 8.1
}

// Retrying a connection at startup, until it succeeds or the context ends.
// Nothing else is calling the dependency yet, so there is no Budget, and
// math.MaxInt attempts leave the context as the only limit. In App.Run the
// context comes from p.NotifyContext, so a SIGTERM ends the wait.
func ExampleNew() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	schedule, err := retry.Exponential(time.Millisecond, 5*time.Millisecond)
	if err != nil {
		fmt.Println(err)
		return
	}
	r, err := retry.New(math.MaxInt, schedule, func(_ context.Context, err error) (retry.Outcome, time.Duration) {
		if err == nil {
			return retry.Success, 0
		}
		return retry.Retry, 0
	}, nil)
	if err != nil {
		fmt.Println(err)
		return
	}

	refused := errors.New("dial tcp 10.0.0.7:5432: connection refused")
	tries := 0
	connect := func(context.Context) error {
		tries++
		if tries < 4 {
			return refused
		}
		return nil
	}

	var connErr error
	var last retry.Decision
	for a := range r.Attempts(ctx) {
		connErr = connect(ctx)
		last = a.Done(connErr)
	}
	if last == retry.Again {
		// The loop ended after an Again without breaking: the context
		// ended during the wait.
		fmt.Println("gave up:", context.Cause(ctx))
		return
	}
	fmt.Printf("connected after %d attempts: %v\n", tries, connErr)
	// Output:
	// connected after 4 attempts: <nil>
}

// A call that must not be made twice, such as a payment, still reports its
// result, so that the Budget counts it, and then breaks out of the loop
// instead of retrying, whatever Done decided.
func ExampleAttempt_Done() {
	r, err := retry.New(3, func(int) time.Duration { return time.Second }, classify, nil)
	if err != nil {
		fmt.Println(err)
		return
	}

	var callErr error
	for a := range r.Attempts(context.Background()) {
		callErr = errors.New("payments: connection reset")
		fmt.Println("decision:", a.Done(callErr))
		break // the payment may have been made
	}
	fmt.Println("call:", callErr)
	// Output:
	// decision: again
	// call: payments: connection reset
}

// A dependency that is down runs the shared Budget down, and from then on
// each call makes one attempt instead of five.
func ExampleNewBudget() {
	budget, err := retry.NewBudget(retry.BudgetConfig{MaxTokens: 4, TokenRatio: 0.5})
	if err != nil {
		fmt.Println(err)
		return
	}
	r, err := retry.New(5, func(int) time.Duration { return 0 }, classify, budget)
	if err != nil {
		fmt.Println(err)
		return
	}

	down := errors.New("inventory: unavailable")
	for i := range 3 {
		var decisions []retry.Decision
		for a := range r.Attempts(context.Background()) {
			decisions = append(decisions, a.Done(down))
		}
		fmt.Printf("call %d: %v, tokens left: %v\n", i+1, decisions, budget.Tokens())
	}
	// Output:
	// call 1: [again throttled], tokens left: 2
	// call 2: [throttled], tokens left: 1
	// call 3: [throttled], tokens left: 0
}

// Configuration that is out of range is an error at startup, naming every
// setting that is wrong.
func ExampleExponential() {
	_, err := retry.Exponential(0, -time.Second)
	fmt.Println(err)
	// Output:
	// retry: base delay must be positive, got 0s
	// retry: max delay must be at least the base delay, 0s, got -1s
}
