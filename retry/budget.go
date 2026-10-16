// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package retry

import (
	"errors"
	"fmt"
	"math"
	"sync"
)

// TokenLimit is the largest BudgetConfig.MaxTokens and
// BudgetConfig.TokenRatio that NewBudget accepts. The bound turns a typo in
// configuration into an error from NewBudget, and keeps the count of tokens
// exact.
const TokenLimit = 1 << 16

// BudgetConfig holds the settings of a Budget that come from configuration.
// Both fields are required: NewBudget rejects a zero or out-of-range value
// rather than replacing it with a default.
type BudgetConfig struct {
	// MaxTokens is how many tokens a full Budget holds, from 3 to
	// TokenLimit. A Budget starts full, and allows no retry while it holds
	// half of MaxTokens or fewer, so a Budget of 1 or 2 tokens, which one
	// failure takes to half, would never allow a retry.
	MaxTokens int

	// TokenRatio is how many tokens each Success adds, from 0.001 to
	// TokenLimit. It is rounded to the nearest thousandth of a token. A
	// TokenRatio of MaxTokens or more refills the Budget on every Success.
	TokenRatio float64
}

// Budget limits the retries of every call that shares it, as gRPC's retry
// throttling does (gRFC A6). It starts with BudgetConfig.MaxTokens tokens.
// Every attempt classified Retry takes one, down to zero, and every attempt
// classified Success adds BudgetConfig.TokenRatio, up to MaxTokens. Stop
// changes nothing. An attempt classified Retry is followed by another only
// while the Budget holds more than half of MaxTokens once its token is
// taken.
//
// Share one Budget between every Retrier that calls the same dependency.
// While the dependency fails, the Budget runs down and retries stop, so a
// failing dependency gets at most one attempt per call instead of one per
// allowed attempt. First attempts are never limited.
//
// Its fields are unexported, so NewBudget is the only way to build one, and
// every method of a Budget not built by NewBudget panics.
type Budget struct {
	// Tokens are counted in thousandths, so that a TokenRatio such as 0.1
	// adds up exactly.
	max, ratio int64

	mu     sync.Mutex
	tokens int64
}

// NewBudget returns a full Budget with the settings cfg. It returns an error
// naming every invalid field of cfg, because cfg comes from configuration
// and App.Run should exit non-zero at startup instead.
func NewBudget(cfg BudgetConfig) (*Budget, error) {
	var errs []error
	if cfg.MaxTokens < 3 || cfg.MaxTokens > TokenLimit {
		errs = append(errs, fmt.Errorf("retry: max tokens must be between 3 and %d, got %d", TokenLimit, cfg.MaxTokens))
	}
	if !(cfg.TokenRatio >= 0.001 && cfg.TokenRatio <= TokenLimit) {
		errs = append(errs, fmt.Errorf("retry: token ratio must be between 0.001 and %d, got %v", TokenLimit, cfg.TokenRatio))
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}

	full := int64(cfg.MaxTokens) * 1000
	return &Budget{
		max:    full,
		ratio:  int64(math.Round(cfg.TokenRatio * 1000)),
		tokens: full,
	}, nil
}

// Tokens returns how many tokens b holds. It changes nothing and calls no
// function of the caller's, so a metrics callback may call it.
//
// Tokens panics if b was not built by NewBudget.
func (b *Budget) Tokens() float64 {
	b.mustBeBuilt("Tokens")

	b.mu.Lock()
	defer b.mu.Unlock()
	return float64(b.tokens) / 1000
}

// succeeded adds TokenRatio tokens to b, up to MaxTokens.
func (b *Budget) succeeded() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.tokens = min(b.max, b.tokens+b.ratio)
}

// failed takes one token from b, down to zero, and reports whether a retry
// may follow.
func (b *Budget) failed() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.tokens = max(0, b.tokens-1000)
	return b.tokens > b.max/2
}

func (b *Budget) mustBeBuilt(method string) {
	if b == nil || b.max == 0 {
		panic("retry: Budget." + method + " called on a Budget not built by NewBudget")
	}
}
