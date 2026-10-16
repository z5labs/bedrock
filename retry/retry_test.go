// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package retry_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/z5labs/bedrock/retry"
)

func TestOutcome_String(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		outcome retry.Outcome
		want    string
	}{
		{name: "success", outcome: retry.Success, want: "success"},
		{name: "retry", outcome: retry.Retry, want: "retry"},
		{name: "stop", outcome: retry.Stop, want: "stop"},
		{name: "zero is not an outcome", outcome: retry.Outcome(0), want: "Outcome(0)"},
		{name: "past the last outcome", outcome: retry.Outcome(4), want: "Outcome(4)"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, tc.outcome.String())
		})
	}
}

func TestDecision_String(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		decision retry.Decision
		want     string
	}{
		{name: "again", decision: retry.Again, want: "again"},
		{name: "succeeded", decision: retry.Succeeded, want: "succeeded"},
		{name: "stopped", decision: retry.Stopped, want: "stopped"},
		{name: "exhausted", decision: retry.Exhausted, want: "exhausted"},
		{name: "throttled", decision: retry.Throttled, want: "throttled"},
		{name: "expired", decision: retry.Expired, want: "expired"},
		{name: "zero is not a decision", decision: retry.Decision(0), want: "Decision(0)"},
		{name: "past the last decision", decision: retry.Decision(7), want: "Decision(7)"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, tc.decision.String())
		})
	}
}
