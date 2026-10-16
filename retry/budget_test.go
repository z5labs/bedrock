// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package retry_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/z5labs/bedrock/retry"
)

func TestNewBudget_rejects_invalid_config(t *testing.T) {
	t.Parallel()

	const (
		maxTokensErr  = "retry: max tokens must be between 3 and 65536, got "
		tokenRatioErr = "retry: token ratio must be between 0.001 and 65536, got "
	)

	testCases := []struct {
		name string
		cfg  retry.BudgetConfig
		want string
	}{
		{
			name: "zero max tokens",
			cfg:  retry.BudgetConfig{MaxTokens: 0, TokenRatio: 0.1},
			want: maxTokensErr + "0",
		},
		{
			name: "max tokens below 3",
			cfg:  retry.BudgetConfig{MaxTokens: 2, TokenRatio: 0.1},
			want: maxTokensErr + "2",
		},
		{
			name: "max tokens above the limit",
			cfg:  retry.BudgetConfig{MaxTokens: retry.TokenLimit + 1, TokenRatio: 0.1},
			want: maxTokensErr + "65537",
		},
		{
			name: "zero token ratio",
			cfg:  retry.BudgetConfig{MaxTokens: 10, TokenRatio: 0},
			want: tokenRatioErr + "0",
		},
		{
			name: "token ratio below a thousandth",
			cfg:  retry.BudgetConfig{MaxTokens: 10, TokenRatio: 0.0009},
			want: tokenRatioErr + "0.0009",
		},
		{
			name: "negative token ratio",
			cfg:  retry.BudgetConfig{MaxTokens: 10, TokenRatio: -1},
			want: tokenRatioErr + "-1",
		},
		{
			name: "NaN token ratio",
			cfg:  retry.BudgetConfig{MaxTokens: 10, TokenRatio: math.NaN()},
			want: tokenRatioErr + "NaN",
		},
		{
			name: "token ratio above the limit",
			cfg:  retry.BudgetConfig{MaxTokens: 10, TokenRatio: retry.TokenLimit + 1},
			want: tokenRatioErr + "65537",
		},
		{
			name: "both fields invalid",
			cfg:  retry.BudgetConfig{MaxTokens: 2, TokenRatio: 0},
			want: maxTokensErr + "2\n" + tokenRatioErr + "0",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b, err := retry.NewBudget(tc.cfg)
			require.Nil(t, b)
			require.EqualError(t, err, tc.want)
		})
	}
}

func TestNewBudget_accepts_the_bounds(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		cfg  retry.BudgetConfig
		want float64
	}{
		{
			name: "lower bounds",
			cfg:  retry.BudgetConfig{MaxTokens: 3, TokenRatio: 0.001},
			want: 3,
		},
		{
			name: "upper bounds",
			cfg:  retry.BudgetConfig{MaxTokens: retry.TokenLimit, TokenRatio: retry.TokenLimit},
			want: retry.TokenLimit,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			b, err := retry.NewBudget(tc.cfg)
			require.NoError(t, err)
			require.NotNil(t, b)
			require.Equal(t, tc.want, b.Tokens())
		})
	}
}

func TestBudget_Tokens_panics_on_a_Budget_not_built_by_NewBudget(t *testing.T) {
	t.Parallel()

	const want = "retry: Budget.Tokens called on a Budget not built by NewBudget"

	testCases := []struct {
		name   string
		budget *retry.Budget
	}{
		{name: "nil", budget: nil},
		{name: "zero", budget: &retry.Budget{}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.PanicsWithValue(t, want, func() {
				_ = tc.budget.Tokens()
			})
		})
	}
}
