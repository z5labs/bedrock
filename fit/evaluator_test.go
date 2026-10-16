// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package fit_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/z5labs/bedrock/fit"
)

// recorder collects every Evaluation it observes. It is safe for concurrent
// use.
type recorder struct {
	mu   sync.Mutex
	evs  []fit.Evaluation
	ctxs []context.Context
}

func (r *recorder) observe(ctx context.Context, ev fit.Evaluation) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.evs = append(r.evs, ev)
	r.ctxs = append(r.ctxs, ctx)
}

func (r *recorder) evaluations() []fit.Evaluation {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]fit.Evaluation(nil), r.evs...)
}

func newEvaluator(t *testing.T, d fit.Decoder, maxDelay time.Duration) (fit.Evaluator, *recorder) {
	t.Helper()

	rec := &recorder{}
	e, err := fit.NewEvaluator(d, maxDelay, rec.observe)
	require.NoError(t, err)
	return e, rec
}

// contextWithScenario returns a copy of ctx carrying s, written by codec.
func contextWithScenario(t *testing.T, ctx context.Context, s fit.Scenario) context.Context {
	t.Helper()

	ctx, err := fit.ContextWithScenario(ctx, codec, s)
	require.NoError(t, err)
	return ctx
}

func TestNewEvaluator(t *testing.T) {
	t.Parallel()

	observe := func(context.Context, fit.Evaluation) {}

	t.Run("panics on a nil decoder", func(t *testing.T) {
		t.Parallel()

		require.Panics(t, func() {
			_, _ = fit.NewEvaluator(nil, time.Second, observe)
		})
	})

	t.Run("panics on a nil observe function", func(t *testing.T) {
		t.Parallel()

		require.Panics(t, func() {
			_, _ = fit.NewEvaluator(codec, time.Second, nil)
		})
	})

	t.Run("returns an error and the zero Evaluator for a max delay that is not positive", func(t *testing.T) {
		t.Parallel()

		for _, maxDelay := range []time.Duration{0, -time.Nanosecond, -time.Second} {
			t.Run(maxDelay.String(), func(t *testing.T) {
				t.Parallel()

				e, err := fit.NewEvaluator(codec, maxDelay, observe)
				require.Error(t, err)
				require.Zero(t, e)
			})
		}
	})

	t.Run("accepts the smallest positive max delay", func(t *testing.T) {
		t.Parallel()

		e, err := fit.NewEvaluator(codec, time.Nanosecond, observe)
		require.NoError(t, err)
		require.NotZero(t, e)
	})
}

func TestEvaluator_Eval(t *testing.T) {
	t.Parallel()

	t.Run("no scenario returns the zero Fault and never calls observe", func(t *testing.T) {
		t.Parallel()

		e, rec := newEvaluator(t, codec, time.Second)
		ctx := contextWithBaggage(t, t.Context(), "tenant=acme")

		require.Zero(t, e.Eval(ctx, "payments"))
		require.Zero(t, e.Eval(t.Context(), "orders"))
		require.Empty(t, rec.evaluations())
	})

	t.Run("rejected scenario returns the zero Fault and reports the decoder's error", func(t *testing.T) {
		t.Parallel()

		decodeErr := errors.New("scenario expired")
		testCases := []struct {
			name string
			ok   bool
		}{
			{name: "with ok set", ok: true},
			{name: "with ok unset", ok: false},
		}
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				d := stubDecoder{
					s:   fit.Scenario{Rules: []fit.Rule{{Point: "*", Fault: fit.Fault{Abort: true}}}},
					ok:  tc.ok,
					err: decodeErr,
				}
				e, rec := newEvaluator(t, d, time.Second)

				require.Zero(t, e.Eval(t.Context(), "payments"))

				evs := rec.evaluations()
				require.Len(t, evs, 1)
				require.Equal(t, "payments", evs[0].Point)
				require.ErrorIs(t, evs[0].Err, decodeErr)
				require.False(t, evs[0].Matched)
				require.Zero(t, evs[0].Rule)
				require.Zero(t, evs[0].Fault)
			})
		}
	})

	t.Run("rejected scenario from a real codec is reported, not returned", func(t *testing.T) {
		t.Parallel()

		e, rec := newEvaluator(t, static, time.Second)
		ctx := contextWithBaggage(t, t.Context(), "chaos.target=payments")

		require.Zero(t, e.Eval(ctx, "payments"))

		evs := rec.evaluations()
		require.Len(t, evs, 1)
		require.ErrorIs(t, evs[0].Err, errStaticMembers)
	})

	t.Run("accepted scenario returns the matching Rule's Fault and reports it", func(t *testing.T) {
		t.Parallel()

		e, rec := newEvaluator(t, codec, time.Second)
		payments := fit.Rule{Point: "payments", Fault: fit.Fault{Delay: 250 * time.Millisecond, Abort: true}}
		ctx := contextWithScenario(t, t.Context(), fit.Scenario{Rules: []fit.Rule{payments}})

		require.Equal(t, payments.Fault, e.Eval(ctx, "payments"))

		evs := rec.evaluations()
		require.Equal(t, []fit.Evaluation{
			{Point: "payments", Rule: payments, Matched: true, Fault: payments.Fault},
		}, evs)
		require.Same(t, ctx, rec.ctxs[0])
	})

	t.Run("accepted scenario reports a point no Rule matches and spares it", func(t *testing.T) {
		t.Parallel()

		e, rec := newEvaluator(t, codec, time.Second)
		ctx := contextWithScenario(t, t.Context(), fit.Scenario{Rules: []fit.Rule{
			{Point: "payments", Fault: fit.Fault{Abort: true}},
		}})

		require.Zero(t, e.Eval(ctx, "orders"))
		require.Equal(t, []fit.Evaluation{{Point: "orders"}}, rec.evaluations())
	})

	t.Run("a Delay over the max is clamped and other points are unaffected", func(t *testing.T) {
		t.Parallel()

		e, rec := newEvaluator(t, codec, time.Second)
		a := fit.Rule{Point: "a", Fault: fit.Fault{Delay: 2 * time.Second}}
		b := fit.Rule{Point: "b", Fault: fit.Fault{Delay: 500 * time.Millisecond, Abort: true}}
		ctx := contextWithScenario(t, t.Context(), fit.Scenario{Rules: []fit.Rule{a, b}})

		require.Equal(t, fit.Fault{Delay: time.Second}, e.Eval(ctx, "a"))
		require.Equal(t, b.Fault, e.Eval(ctx, "b"))

		require.Equal(t, []fit.Evaluation{
			{Point: "a", Rule: a, Matched: true, Fault: fit.Fault{Delay: time.Second}},
			{Point: "b", Rule: b, Matched: true, Fault: b.Fault},
		}, rec.evaluations())
	})

	t.Run("a Delay equal to the max is not changed", func(t *testing.T) {
		t.Parallel()

		e, _ := newEvaluator(t, codec, time.Second)
		ctx := contextWithScenario(t, t.Context(), fit.Scenario{Rules: []fit.Rule{
			{Point: "a", Fault: fit.Fault{Delay: time.Second, Abort: true}},
		}})

		require.Equal(t, fit.Fault{Delay: time.Second, Abort: true}, e.Eval(ctx, "a"))
	})

	t.Run("scenario with no rules injects nothing and reports every point", func(t *testing.T) {
		t.Parallel()

		e, rec := newEvaluator(t, codec, time.Second)
		ctx := contextWithScenario(t, t.Context(), fit.Scenario{})

		points := []string{"http:payments", "grpc:orders", "sql:accounts"}
		for _, p := range points {
			require.Zero(t, e.Eval(ctx, p))
		}

		evs := rec.evaluations()
		require.Len(t, evs, len(points))
		for i, p := range points {
			require.Equal(t, fit.Evaluation{Point: p}, evs[i])
			require.False(t, evs[i].Matched)
		}
	})

	t.Run("is safe to share between goroutines", func(t *testing.T) {
		t.Parallel()

		e, rec := newEvaluator(t, codec, time.Second)
		ctx := contextWithScenario(t, t.Context(), fit.Scenario{Rules: []fit.Rule{
			{Point: "a", Fault: fit.Fault{Delay: 2 * time.Second, Abort: true}},
		}})
		rejected := contextWithBaggage(t, t.Context(), "chaos.scenario=not-json")

		const goroutines = 64
		var wg sync.WaitGroup
		faults := make([]fit.Fault, goroutines)
		for i := range goroutines {
			wg.Go(func() {
				switch i % 3 {
				case 0:
					faults[i] = e.Eval(ctx, "a")
				case 1:
					faults[i] = e.Eval(ctx, fmt.Sprintf("b%d", i))
				default:
					faults[i] = e.Eval(rejected, "a")
				}
			})
		}
		wg.Wait()

		for i, f := range faults {
			if i%3 == 0 {
				require.Equal(t, fit.Fault{Delay: time.Second, Abort: true}, f)
				continue
			}
			require.Zero(t, f)
		}
		require.Len(t, rec.evaluations(), goroutines)
	})
}
