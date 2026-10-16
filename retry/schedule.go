// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package retry

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"time"
)

// Exponential returns a schedule for [New] that backs off exponentially with
// full jitter: the delay after attempt n is drawn uniformly from
// [0, min(maxDelay, base·2ⁿ⁻¹)). Full jitter spreads out the retries of
// callers that failed together, so they do not all try again at once.
//
// The schedule draws from math/rand/v2's top-level functions, so it is safe
// for concurrent use, and one schedule may be shared by every call. It
// panics if it is called with an attempt less than 1.
//
// Exponential returns an error naming every invalid argument, because both
// come from configuration: base must be positive, and maxDelay at least
// base.
func Exponential(base, maxDelay time.Duration) (func(attempt int) time.Duration, error) {
	var errs []error
	if base <= 0 {
		errs = append(errs, fmt.Errorf("retry: base delay must be positive, got %v", base))
	}
	if maxDelay < base {
		errs = append(errs, fmt.Errorf("retry: max delay must be at least the base delay, %v, got %v", base, maxDelay))
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}

	return func(attempt int) time.Duration {
		if attempt < 1 {
			panic(fmt.Sprintf("retry: schedule called with attempt %d, which is less than 1", attempt))
		}
		// base<<(attempt-1) is at most maxDelay whenever base is at most
		// maxDelay>>(attempt-1), so the shift never overflows.
		ceiling := maxDelay
		if base <= maxDelay>>(attempt-1) {
			ceiling = base << (attempt - 1)
		}
		return rand.N(ceiling)
	}, nil
}
