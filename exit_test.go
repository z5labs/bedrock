// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package bedrock_test

import (
	"errors"
	"math"
	"strconv"
	"testing"

	"github.com/z5labs/bedrock"

	"github.com/stretchr/testify/require"
)

// panicValue calls f and returns the value it panicked with, or nil if it
// returned normally. Recovering is how a test observes a panic; library
// code never recovers one.
func panicValue(f func()) (v any) {
	defer func() {
		v = recover()
	}()
	f()
	return nil
}

func TestExit(t *testing.T) {
	t.Parallel()

	t.Run("panics on a code outside 0..255", func(t *testing.T) {
		t.Parallel()

		for _, code := range []int{-1, 256, math.MaxInt, math.MinInt} {
			t.Run(strconv.Itoa(code), func(t *testing.T) {
				t.Parallel()

				v := panicValue(func() { bedrock.Exit(code) })
				require.NotNil(t, v)

				err, ok := v.(error)
				require.True(t, ok, "panic value %#v is not an error", v)
				require.ErrorContains(t, err, strconv.Itoa(code))
			})
		}
	})

	t.Run("returns any code in 0..255 with no cause", func(t *testing.T) {
		t.Parallel()

		for code := 0; code <= 255; code++ {
			status := bedrock.Exit(code)
			require.Equal(t, code, status.Code())
			require.NoError(t, status.Cause())
		}
	})
}

func TestExitWithCause(t *testing.T) {
	t.Parallel()

	t.Run("panics on a code outside 0..255", func(t *testing.T) {
		t.Parallel()

		for _, code := range []int{-1, 256, math.MaxInt, math.MinInt} {
			t.Run(strconv.Itoa(code), func(t *testing.T) {
				t.Parallel()

				t.Run("with a cause", func(t *testing.T) {
					t.Parallel()

					cause := errors.New("the cause")
					v := panicValue(func() { bedrock.ExitWithCause(code, cause) })
					require.NotNil(t, v)

					err, ok := v.(error)
					require.True(t, ok, "panic value %#v is not an error", v)
					require.ErrorIs(t, err, cause)
					require.ErrorContains(t, err, strconv.Itoa(code))
				})

				t.Run("with a nil cause", func(t *testing.T) {
					t.Parallel()

					v := panicValue(func() { bedrock.ExitWithCause(code, nil) })
					require.NotNil(t, v)

					err, ok := v.(error)
					require.True(t, ok, "panic value %#v is not an error", v)
					require.ErrorContains(t, err, strconv.Itoa(code))
				})
			})
		}
	})

	t.Run("returns any code in 0..255 with its cause", func(t *testing.T) {
		t.Parallel()

		for code := 0; code <= 255; code++ {
			cause := errors.New("the cause")
			status := bedrock.ExitWithCause(code, cause)
			require.Equal(t, code, status.Code())
			require.Equal(t, cause, status.Cause())
		}
	})

	t.Run("returns any code in 0..255 with a nil cause", func(t *testing.T) {
		t.Parallel()

		for code := 0; code <= 255; code++ {
			status := bedrock.ExitWithCause(code, nil)
			require.Equal(t, code, status.Code())
			require.NoError(t, status.Cause())
		}
	})
}

func TestExitStatus(t *testing.T) {
	t.Parallel()

	t.Run("the zero value is success with no cause", func(t *testing.T) {
		t.Parallel()

		var status bedrock.ExitStatus
		require.Equal(t, 0, status.Code())
		require.NoError(t, status.Cause())
	})
}
