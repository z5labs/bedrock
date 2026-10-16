// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package circuit_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/z5labs/bedrock/circuit"
)

func ExampleBreaker_Allow() {
	b, err := circuit.New(
		circuit.Config{Window: 20, Probes: 1, OpenFor: 30 * time.Second, ProbeTimeout: 5 * time.Second},
		circuit.Policy{
			Trip: func(c circuit.Counts) bool { return c.ConsecutiveFailures >= 3 },
			Probe: func(c circuit.Counts) circuit.State {
				switch {
				case c.Failures > 0 || c.TimedOut > 0:
					return circuit.Open
				case c.Successes > 0:
					return circuit.Closed
				default:
					return circuit.HalfOpen
				}
			},
		},
		// Ignore only a call its caller cancelled. A deadline that runs
		// out is a failure, or a hung dependency would never trip.
		func(ctx context.Context, err error) circuit.Outcome {
			switch {
			case err == nil:
				return circuit.Success
			case errors.Is(ctx.Err(), context.Canceled):
				return circuit.Ignored
			default:
				return circuit.Failure
			}
		},
		func(_ context.Context, t circuit.Transition) {
			fmt.Printf("%v -> %v\n", t.From, t.To)
		},
	)
	if err != nil {
		fmt.Println(err)
		return
	}

	callPayments := func(context.Context) error { return errors.New("connection refused") }

	ctx := context.Background()
	for range 4 {
		err := func() error {
			t, err := b.Allow(ctx)
			if err != nil {
				return err // a *circuit.RejectedError
			}
			defer t.Release()

			err = callPayments(ctx)
			t.Done(err)
			return err
		}()
		if errors.Is(err, circuit.ErrRejected) {
			fmt.Println("rejected: the breaker is open")
			continue
		}
		fmt.Println(err)
	}
	// Output:
	// connection refused
	// connection refused
	// closed -> open
	// connection refused
	// rejected: the breaker is open
}
