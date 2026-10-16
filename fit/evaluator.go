// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package fit

import (
	"context"
	"fmt"
	"time"
)

// Evaluation is what Evaluator.Eval reports for one injection point.
type Evaluation struct {
	Point   string
	Rule    Rule // the matching Rule, if Matched
	Matched bool
	Fault   Fault // what the point emulates, its Delay clamped to the maximum
	Err     error // why the carried Scenario was ignored, if it was
}

// Evaluator evaluates the Scenario a request carries at named injection
// points. Build one with NewEvaluator at startup and share it between
// requests. Its fields are unexported, so NewEvaluator is the only way to
// build one.
type Evaluator struct {
	decoder  Decoder
	maxDelay time.Duration
	observe  func(context.Context, Evaluation)
}

// NewEvaluator returns an Evaluator that reads Scenarios with d, honours no
// Delay longer than maxDelay, and reports every evaluation of a request that
// carries a Scenario to observe.
//
// It panics if d or observe is nil, which is a programming error. It returns
// an error if maxDelay is not positive, because maxDelay comes from
// configuration and App.Run should exit non-zero at startup instead.
func NewEvaluator(d Decoder, maxDelay time.Duration, observe func(context.Context, Evaluation)) (Evaluator, error) {
	if d == nil {
		panic("fit: NewEvaluator called with a nil Decoder")
	}
	if observe == nil {
		panic("fit: NewEvaluator called with a nil observe function")
	}
	if maxDelay <= 0 {
		return Evaluator{}, fmt.Errorf("fit: max delay must be positive, got %v", maxDelay)
	}
	return Evaluator{decoder: d, maxDelay: maxDelay, observe: observe}, nil
}

// Eval returns the Fault the injection point named point emulates for the
// request ctx belongs to.
//
// A request that carries no Scenario gets the zero Fault, and observe is not
// called. A request whose Scenario the Decoder rejects also gets the zero
// Fault, and observe receives the Decoder's error in Evaluation.Err: test
// metadata never fails a request on its own. Otherwise the first matching
// Rule decides the Fault, its Delay is clamped to the Evaluator's maximum,
// and observe receives the result whether or not a Rule matched.
func (e Evaluator) Eval(ctx context.Context, point string) Fault {
	s, ok, err := ScenarioFromContext(ctx, e.decoder)
	if err != nil {
		e.observe(ctx, Evaluation{Point: point, Err: err})
		return Fault{}
	}
	if !ok {
		return Fault{}
	}

	r, matched := s.Match(point)
	f := r.Fault
	f.Delay = min(f.Delay, e.maxDelay)
	e.observe(ctx, Evaluation{Point: point, Rule: r, Matched: matched, Fault: f})
	return f
}
