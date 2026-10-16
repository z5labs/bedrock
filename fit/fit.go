// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package fit

import (
	"context"
	"errors"
	"strings"
	"time"
)

// Fault is a failure an injection point emulates. The zero Fault emulates nothing.
type Fault struct {
	Delay time.Duration // how long the injection point waits before it proceeds or aborts
	Abort bool          // the injection point fails instead of proceeding
}

// ErrInjected is the error Fault.Inject returns for an aborting Fault.
var ErrInjected = errors.New("fit: injected failure")

// Wait waits out f.Delay. It returns context.Cause(ctx) if ctx is done first.
// A Delay of zero or less returns nil at once.
func (f Fault) Wait(ctx context.Context) error {
	if f.Delay <= 0 {
		return nil
	}
	timer := time.NewTimer(f.Delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}

// Inject emulates f generically: it calls Wait, then returns ErrInjected if
// f.Abort. An injection point that can fail more realistically, such as with a
// 503 or codes.Unavailable, calls Wait and checks Abort itself.
func (f Fault) Inject(ctx context.Context) error {
	if err := f.Wait(ctx); err != nil {
		return err
	}
	if f.Abort {
		return ErrInjected
	}
	return nil
}

// Rule matches injection points by name and gives the Fault they emulate.
type Rule struct {
	// Point is the name of an injection point, a prefix of one followed by
	// "*", or a lone "*", which matches every point. fit puts no limit on the
	// characters a name may contain.
	Point string

	// Fault is what matching points emulate. The zero Fault spares them.
	Fault Fault
}

// matches reports whether r.Point names point. Only a trailing "*" is a
// wildcard; a "*" anywhere else is an ordinary character of the name.
func (r Rule) matches(point string) bool {
	prefix, wildcard := strings.CutSuffix(r.Point, "*")
	if !wildcard {
		return r.Point == point
	}
	return strings.HasPrefix(point, prefix)
}

// Scenario is a set of injection points that should or should not fail.
type Scenario struct {
	// Rules are tried in order, and the first match decides a point's Fault.
	// A point no Rule matches emulates nothing.
	Rules []Rule
}

// Match returns the first Rule of s that matches point.
func (s Scenario) Match(point string) (Rule, bool) {
	for _, r := range s.Rules {
		if r.matches(point) {
			return r, true
		}
	}
	return Rule{}, false
}
