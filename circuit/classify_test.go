// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package circuit_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/z5labs/bedrock/circuit"
)

type ctxKey struct{}

func TestBreaker_classification(t *testing.T) {
	t.Parallel()

	t.Run("classify receives Allow's context and Done's error, and Release does not call it", func(t *testing.T) {
		t.Parallel()

		var (
			mu   sync.Mutex
			ctxs []context.Context
			errs []error
		)
		classify := func(ctx context.Context, err error) circuit.Outcome {
			mu.Lock()
			defer mu.Unlock()
			ctxs = append(ctxs, ctx)
			errs = append(errs, err)
			return circuit.Success
		}
		b := newBreaker(t, testConfig(), circuit.Policy{Trip: neverTrip, Probe: stayHalfOpen}, classify, ignoreTransitions)

		ctx := context.WithValue(t.Context(), ctxKey{}, "allow")
		tk := mustAllow(t, b, ctx)
		want := errors.New("from the call")
		require.True(t, tk.Done(want))

		released := mustAllow(t, b, ctx)
		released.Release()

		mu.Lock()
		defer mu.Unlock()
		require.Len(t, ctxs, 1)
		require.Equal(t, "allow", ctxs[0].Value(ctxKey{}))
		require.Len(t, errs, 1)
		require.ErrorIs(t, errs[0], want)
	})

	t.Run("no error is treated specially", func(t *testing.T) {
		t.Parallel()

		inverted := func(_ context.Context, err error) circuit.Outcome {
			if err == nil {
				return circuit.Failure
			}
			return circuit.Success
		}
		b := newBreaker(t, testConfig(), circuit.Policy{Trip: tripOn(1), Probe: stayHalfOpen}, inverted, ignoreTransitions)

		require.True(t, settle(t, b, t.Context(), nil))
		require.Equal(t, circuit.Open, b.State())
	})

	t.Run("the recommended classifier", func(t *testing.T) {
		t.Parallel()

		t.Run("ignores a cancelled call", func(t *testing.T) {
			t.Parallel()

			pol := &policyRecorder{trip: tripOn(1), probe: stayHalfOpen}
			b := newBreaker(t, testConfig(), pol.policy(), recommendedClassifier, ignoreTransitions)

			ctx, cancel := context.WithCancel(t.Context())
			tk := mustAllow(t, b, ctx)
			defer tk.Release()
			cancel()
			require.True(t, tk.Done(ctx.Err()))

			require.Empty(t, pol.tripCounts())
			require.Equal(t, circuit.Closed, b.State())
		})

		t.Run("counts the caller's deadline as a failure that opens the breaker", func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				b := newBreaker(t, testConfig(), circuit.Policy{Trip: tripOn(1), Probe: stayHalfOpen}, recommendedClassifier, ignoreTransitions)

				ctx, cancel := context.WithTimeout(t.Context(), time.Second)
				defer cancel()
				tk := mustAllow(t, b, ctx)
				defer tk.Release()

				<-ctx.Done() // the call hangs until the caller's deadline
				require.True(t, tk.Done(ctx.Err()))

				require.Equal(t, circuit.Open, b.State())
			})
		})

		t.Run("counts a deadline set inside the call as a failure", func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				b := newBreaker(t, testConfig(), circuit.Policy{Trip: tripOn(1), Probe: stayHalfOpen}, recommendedClassifier, ignoreTransitions)

				ctx := t.Context()
				tk := mustAllow(t, b, ctx)
				defer tk.Release()

				err := func() error {
					inner, cancel := context.WithTimeout(ctx, time.Second)
					defer cancel()
					<-inner.Done()
					return fmt.Errorf("calling payments: %w", inner.Err())
				}()
				require.NoError(t, ctx.Err())
				require.True(t, tk.Done(err))

				require.Equal(t, circuit.Open, b.State())
			})
		})
	})
}
