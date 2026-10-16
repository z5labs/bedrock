// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package circuit_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/z5labs/bedrock/circuit"
)

func TestState_String(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		state circuit.State
		want  string
	}{
		{name: "closed", state: circuit.Closed, want: "closed"},
		{name: "open", state: circuit.Open, want: "open"},
		{name: "half-open", state: circuit.HalfOpen, want: "half-open"},
		{name: "zero is not a state", state: circuit.State(0), want: "State(0)"},
		{name: "past the last state", state: circuit.State(4), want: "State(4)"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, tc.state.String())
		})
	}
}

func TestOutcome_String(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		outcome circuit.Outcome
		want    string
	}{
		{name: "success", outcome: circuit.Success, want: "success"},
		{name: "failure", outcome: circuit.Failure, want: "failure"},
		{name: "ignored", outcome: circuit.Ignored, want: "ignored"},
		{name: "zero is not an outcome", outcome: circuit.Outcome(0), want: "Outcome(0)"},
		{name: "past the last outcome", outcome: circuit.Outcome(4), want: "Outcome(4)"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, tc.outcome.String())
		})
	}
}

func TestRejectedError_Error(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		err  *circuit.RejectedError
		want string
	}{
		{
			name: "open with a retry after",
			err:  &circuit.RejectedError{State: circuit.Open, RetryAfter: 4200 * time.Millisecond},
			want: "circuit: call rejected: open for another 4.2s",
		},
		{
			name: "half-open",
			err:  &circuit.RejectedError{State: circuit.HalfOpen},
			want: "circuit: call rejected: half-open, and every probe slot is taken",
		},
		{
			name: "unknown state",
			err:  &circuit.RejectedError{State: circuit.State(7)},
			want: "circuit: call rejected: State(7)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.EqualError(t, tc.err, tc.want)
		})
	}
}

func TestRejectedError_Unwrap(t *testing.T) {
	t.Parallel()

	err := &circuit.RejectedError{State: circuit.Open, RetryAfter: time.Second}

	require.Equal(t, circuit.ErrRejected, err.Unwrap())
}

func TestRejectedError_matchesThroughWrapping(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		err  *circuit.RejectedError
	}{
		{
			name: "open",
			err:  &circuit.RejectedError{State: circuit.Open, RetryAfter: time.Second},
		},
		{
			name: "half-open",
			err:  &circuit.RejectedError{State: circuit.HalfOpen},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			wrapped := fmt.Errorf("calling payments: %w", tc.err)

			require.ErrorIs(t, wrapped, circuit.ErrRejected)

			got, ok := errors.AsType[*circuit.RejectedError](wrapped)
			require.True(t, ok)
			require.Same(t, tc.err, got)
		})
	}
}
