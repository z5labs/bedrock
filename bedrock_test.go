// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package bedrock

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var (
	// Signal handler setup time - matches Go stdlib default
	signalSettleTime = 100 * time.Millisecond
	// Timeout for signal delivery - absurdly long to prevent CI flakes
	signalWaitTimeout = 5 * time.Second
)

func init() {
	// Honor GO_TEST_TIMEOUT_SCALE for slow CI builders (matches Go stdlib pattern)
	if s := os.Getenv("GO_TEST_TIMEOUT_SCALE"); s != "" {
		if scale, err := strconv.Atoi(s); err == nil && scale > 0 {
			signalSettleTime *= time.Duration(scale)
			signalWaitTimeout *= time.Duration(scale)
		}
	}
}

// quiesce waits for signal handlers to be ready.
// Splits sleep into chunks to give kernel multiple delivery opportunities.
func quiesce() {
	start := time.Now()
	for time.Since(start) < signalSettleTime {
		time.Sleep(signalSettleTime / 10)
	}
}

func TestBuilderFunc_Build(t *testing.T) {
	testCases := []struct {
		name        string
		builder     BuilderFunc[int]
		expectedVal int
		expectErr   bool
	}{
		{
			name: "successfully builds value",
			builder: BuilderFunc[int](func(ctx context.Context) (int, error) {
				return 42, nil
			}),
			expectedVal: 42,
		},
		{
			name: "propagates build error",
			builder: BuilderFunc[int](func(ctx context.Context) (int, error) {
				return 0, errors.New("build failed")
			}),
			expectErr: true,
		},
		{
			name: "respects context",
			builder: BuilderFunc[int](func(ctx context.Context) (int, error) {
				if ctx == nil {
					return 0, errors.New("context is nil")
				}
				return 99, nil
			}),
			expectedVal: 99,
		},
		{
			name: "returns value even on error",
			builder: BuilderFunc[int](func(ctx context.Context) (int, error) {
				return 100, errors.New("error occurred")
			}),
			expectedVal: 100,
			expectErr:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			val, err := tc.builder.Build(context.Background())
			if tc.expectErr {
				require.Error(t, err)
				require.Equal(t, tc.expectedVal, val)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expectedVal, val)
			}
		})
	}
}

func TestMap(t *testing.T) {
	testCases := []struct {
		name        string
		builder     Builder[int]
		mapper      func(int) (string, error)
		expectedVal string
		expectErr   bool
		expectMapperCalled bool
	}{
		{
			name: "maps value successfully",
			builder: BuilderFunc[int](func(ctx context.Context) (int, error) {
				return 42, nil
			}),
			mapper: func(i int) (string, error) {
				return fmt.Sprintf("%d", i), nil
			},
			expectedVal: "42",
			expectMapperCalled: true,
		},
		{
			name: "propagates builder error",
			builder: BuilderFunc[int](func(ctx context.Context) (int, error) {
				return 0, errors.New("builder failed")
			}),
			mapper: func(i int) (string, error) {
				return "should not be called", nil
			},
			expectErr: true,
			expectMapperCalled: false,
		},
		{
			name: "propagates mapper error",
			builder: BuilderFunc[int](func(ctx context.Context) (int, error) {
				return 5, nil
			}),
			mapper: func(i int) (string, error) {
				return "", errors.New("mapper failed")
			},
			expectErr: true,
			expectMapperCalled: true,
		},
		{
			name: "maps to different type",
			builder: BuilderFunc[int](func(ctx context.Context) (int, error) {
				return 100, nil
			}),
			mapper: func(i int) (string, error) {
				return fmt.Sprintf("value:%d", i), nil
			},
			expectedVal: "value:100",
			expectMapperCalled: true,
		},
		{
			name: "returns zero value of target type on builder error",
			builder: BuilderFunc[int](func(ctx context.Context) (int, error) {
				return 999, errors.New("error")
			}),
			mapper: func(i int) (string, error) {
				return "not called", nil
			},
			expectedVal: "",
			expectErr: true,
			expectMapperCalled: false,
		},
		{
			name: "passes context to builder",
			builder: BuilderFunc[int](func(ctx context.Context) (int, error) {
				if ctx == nil {
					return 0, errors.New("context is nil")
				}
				return 7, nil
			}),
			mapper: func(i int) (string, error) {
				return fmt.Sprintf("%d", i), nil
			},
			expectedVal: "7",
			expectMapperCalled: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mapperCalled := false
			wrappedMapper := func(i int) (string, error) {
				mapperCalled = true
				return tc.mapper(i)
			}

			mapped := Map(tc.builder, wrappedMapper)
			val, err := mapped.Build(context.Background())

			if tc.expectErr {
				require.Error(t, err)
				require.Zero(t, val)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expectedVal, val)
			}

			require.Equal(t, tc.expectMapperCalled, mapperCalled, "mapper called status mismatch")
		})
	}
}

func TestBind(t *testing.T) {
	testCases := []struct {
		name        string
		builder     Builder[string]
		binder      func(string) Builder[int]
		expectedVal int
		expectErr   bool
		expectBinderCalled bool
	}{
		{
			name: "chains builders successfully",
			builder: BuilderFunc[string](func(ctx context.Context) (string, error) {
				return "key", nil
			}),
			binder: func(s string) Builder[int] {
				return BuilderFunc[int](func(ctx context.Context) (int, error) {
					if s == "key" {
						return 42, nil
					}
					return 0, errors.New("unexpected key")
				})
			},
			expectedVal: 42,
			expectBinderCalled: true,
		},
		{
			name: "propagates first builder error",
			builder: BuilderFunc[string](func(ctx context.Context) (string, error) {
				return "", errors.New("first builder failed")
			}),
			binder: func(s string) Builder[int] {
				return BuilderFunc[int](func(ctx context.Context) (int, error) {
					return 99, errors.New("should not be called")
				})
			},
			expectErr: true,
			expectBinderCalled: false,
		},
		{
			name: "propagates second builder error",
			builder: BuilderFunc[string](func(ctx context.Context) (string, error) {
				return "test", nil
			}),
			binder: func(s string) Builder[int] {
				return BuilderFunc[int](func(ctx context.Context) (int, error) {
					return 0, errors.New("second builder failed")
				})
			},
			expectErr: true,
			expectBinderCalled: true,
		},
		{
			name: "binder receives first builder output",
			builder: BuilderFunc[string](func(ctx context.Context) (string, error) {
				return "correct-value", nil
			}),
			binder: func(s string) Builder[int] {
				return BuilderFunc[int](func(ctx context.Context) (int, error) {
					if s == "correct-value" {
						return 123, nil
					}
					return 0, errors.New("binder received wrong value")
				})
			},
			expectedVal: 123,
			expectBinderCalled: true,
		},
		{
			name: "second builder receives context",
			builder: BuilderFunc[string](func(ctx context.Context) (string, error) {
				return "data", nil
			}),
			binder: func(s string) Builder[int] {
				return BuilderFunc[int](func(ctx context.Context) (int, error) {
					if ctx == nil {
						return 0, errors.New("context is nil")
					}
					return 77, nil
				})
			},
			expectedVal: 77,
			expectBinderCalled: true,
		},
		{
			name: "returns zero value of type B on first error",
			builder: BuilderFunc[string](func(ctx context.Context) (string, error) {
				return "", errors.New("first failed")
			}),
			binder: func(s string) Builder[int] {
				return BuilderFunc[int](func(ctx context.Context) (int, error) {
					return 999, nil
				})
			},
			expectedVal: 0,
			expectErr: true,
			expectBinderCalled: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			binderCalled := false
			wrappedBinder := func(s string) Builder[int] {
				binderCalled = true
				return tc.binder(s)
			}

			bound := Bind(tc.builder, wrappedBinder)
			val, err := bound.Build(context.Background())

			if tc.expectErr {
				require.Error(t, err)
				require.Zero(t, val)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expectedVal, val)
			}

			require.Equal(t, tc.expectBinderCalled, binderCalled, "binder called status mismatch")
		})
	}
}

func TestRuntimeFunc_Run(t *testing.T) {
	testCases := []struct {
		name      string
		runtime   RuntimeFunc
		expectErr bool
	}{
		{
			name: "executes successfully",
			runtime: RuntimeFunc(func(ctx context.Context) error {
				return nil
			}),
		},
		{
			name: "propagates runtime error",
			runtime: RuntimeFunc(func(ctx context.Context) error {
				return errors.New("runtime failed")
			}),
			expectErr: true,
		},
		{
			name: "receives context",
			runtime: RuntimeFunc(func(ctx context.Context) error {
				if ctx == nil {
					return errors.New("context is nil")
				}
				return nil
			}),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.runtime.Run(context.Background())
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestRunnerFunc_Run(t *testing.T) {
	testCases := []struct {
		name      string
		runner    RunnerFunc[Runtime]
		builder   Builder[Runtime]
		expectErr bool
	}{
		{
			name: "executes successfully",
			runner: RunnerFunc[Runtime](func(ctx context.Context, builder Builder[Runtime]) error {
				return nil
			}),
			builder: BuilderFunc[Runtime](func(ctx context.Context) (Runtime, error) {
				return RuntimeFunc(func(ctx context.Context) error {
					return nil
				}), nil
			}),
		},
		{
			name: "propagates runner error",
			runner: RunnerFunc[Runtime](func(ctx context.Context, builder Builder[Runtime]) error {
				return errors.New("runner failed")
			}),
			builder: BuilderFunc[Runtime](func(ctx context.Context) (Runtime, error) {
				return RuntimeFunc(func(ctx context.Context) error {
					return nil
				}), nil
			}),
			expectErr: true,
		},
		{
			name: "receives context and builder",
			runner: RunnerFunc[Runtime](func(ctx context.Context, builder Builder[Runtime]) error {
				if ctx == nil {
					return errors.New("context is nil")
				}
				if builder == nil {
					return errors.New("builder is nil")
				}
				return nil
			}),
			builder: BuilderFunc[Runtime](func(ctx context.Context) (Runtime, error) {
				return RuntimeFunc(func(ctx context.Context) error {
					return nil
				}), nil
			}),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.runner.Run(context.Background(), tc.builder)
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestDefaultRunner(t *testing.T) {
	testCases := []struct {
		name      string
		builder   Builder[Runtime]
		expectErr bool
	}{
		{
			name: "builds and runs successfully",
			builder: BuilderFunc[Runtime](func(ctx context.Context) (Runtime, error) {
				return RuntimeFunc(func(ctx context.Context) error {
					return nil
				}), nil
			}),
		},
		{
			name: "propagates builder error",
			builder: BuilderFunc[Runtime](func(ctx context.Context) (Runtime, error) {
				return nil, errors.New("builder failed")
			}),
			expectErr: true,
		},
		{
			name: "propagates runtime error",
			builder: BuilderFunc[Runtime](func(ctx context.Context) (Runtime, error) {
				return RuntimeFunc(func(ctx context.Context) error {
					return errors.New("runtime failed")
				}), nil
			}),
			expectErr: true,
		},
		{
			name: "passes context to builder",
			builder: BuilderFunc[Runtime](func(ctx context.Context) (Runtime, error) {
				if ctx == nil {
					return nil, errors.New("builder context is nil")
				}
				return RuntimeFunc(func(ctx context.Context) error {
					return nil
				}), nil
			}),
		},
		{
			name: "passes context to runtime",
			builder: BuilderFunc[Runtime](func(ctx context.Context) (Runtime, error) {
				return RuntimeFunc(func(ctx context.Context) error {
					if ctx == nil {
						return errors.New("runtime context is nil")
					}
					return nil
				}), nil
			}),
		},
		{
			name: "calls Build before Run",
			builder: BuilderFunc[Runtime](func(ctx context.Context) (Runtime, error) {
				buildCalled := true
				return RuntimeFunc(func(ctx context.Context) error {
					if !buildCalled {
						return errors.New("Run called before Build")
					}
					return nil
				}), nil
			}),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			runner := DefaultRunner[Runtime]()
			err := runner.Run(context.Background(), tc.builder)
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestNotifyOnSignal(t *testing.T) {
	testCases := []struct {
		name      string
		builder   Builder[Runtime]
		signals   []os.Signal
		expectErr bool
		skipSignalTest bool
	}{
		{
			name: "wraps runner successfully",
			builder: BuilderFunc[Runtime](func(ctx context.Context) (Runtime, error) {
				return RuntimeFunc(func(ctx context.Context) error {
					return nil
				}), nil
			}),
			signals: []os.Signal{os.Interrupt},
		},
		{
			name: "propagates runner error",
			builder: BuilderFunc[Runtime](func(ctx context.Context) (Runtime, error) {
				return RuntimeFunc(func(ctx context.Context) error {
					return errors.New("runtime failed")
				}), nil
			}),
			signals: []os.Signal{os.Interrupt},
			expectErr: true,
		},
		{
			name: "passes context to wrapped runner",
			builder: BuilderFunc[Runtime](func(ctx context.Context) (Runtime, error) {
				if ctx == nil {
					return nil, errors.New("context is nil")
				}
				return RuntimeFunc(func(ctx context.Context) error {
					if ctx == nil {
						return errors.New("runtime context is nil")
					}
					return nil
				}), nil
			}),
			signals: []os.Signal{os.Interrupt},
		},
		{
			name: "context cancels when parent context cancelled",
			builder: BuilderFunc[Runtime](func(ctx context.Context) (Runtime, error) {
				return RuntimeFunc(func(ctx context.Context) error {
					<-ctx.Done()
					return ctx.Err()
				}), nil
			}),
			signals: []os.Signal{os.Interrupt},
			expectErr: true,
		},
		{
			name: "handles empty signal list",
			builder: BuilderFunc[Runtime](func(ctx context.Context) (Runtime, error) {
				return RuntimeFunc(func(ctx context.Context) error {
					return nil
				}), nil
			}),
			signals: []os.Signal{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			runner := NotifyOnSignal(
				DefaultRunner[Runtime](),
				tc.signals...,
			)

			var err error
			if tc.name == "context cancels when parent context cancelled" {
				ctx, cancel := context.WithCancel(context.Background())
				go func() {
					time.Sleep(50 * time.Millisecond)
					cancel()
				}()
				err = runner.Run(ctx, tc.builder)
			} else {
				err = runner.Run(context.Background(), tc.builder)
			}

			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}

	// Actual signal tests
	t.Run("cancels context when signal received", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("signals work differently on Windows")
		}

		signalReceived := make(chan struct{})

		builder := BuilderFunc[Runtime](func(ctx context.Context) (Runtime, error) {
			return RuntimeFunc(func(ctx context.Context) error {
				<-ctx.Done()
				close(signalReceived)
				return ctx.Err()
			}), nil
		})

		runner := NotifyOnSignal(
			DefaultRunner[Runtime](),
			syscall.SIGUSR1,
		)

		go func() {
			quiesce() // Wait for signal handler setup
			syscall.Kill(syscall.Getpid(), syscall.SIGUSR1)
		}()

		err := runner.Run(context.Background(), builder)

		select {
		case <-signalReceived:
			require.ErrorIs(t, err, context.Canceled)
		case <-time.After(signalWaitTimeout):
			t.Fatalf("timeout after %v waiting for signal", signalWaitTimeout)
		}
	})

	t.Run("handles multiple signals", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("signals work differently on Windows")
		}

		signalReceived := make(chan struct{})

		builder := BuilderFunc[Runtime](func(ctx context.Context) (Runtime, error) {
			return RuntimeFunc(func(ctx context.Context) error {
				<-ctx.Done()
				close(signalReceived)
				return ctx.Err()
			}), nil
		})

		runner := NotifyOnSignal(
			DefaultRunner[Runtime](),
			syscall.SIGUSR1,
			syscall.SIGUSR2,
		)

		go func() {
			quiesce() // Wait for signal handler setup
			syscall.Kill(syscall.Getpid(), syscall.SIGUSR2)
		}()

		err := runner.Run(context.Background(), builder)

		select {
		case <-signalReceived:
			require.ErrorIs(t, err, context.Canceled)
		case <-time.After(signalWaitTimeout):
			t.Fatalf("timeout after %v waiting for signal", signalWaitTimeout)
		}
	})

	t.Run("returns context.Canceled error when signal received", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("signals work differently on Windows")
		}

		builder := BuilderFunc[Runtime](func(ctx context.Context) (Runtime, error) {
			return RuntimeFunc(func(ctx context.Context) error {
				<-ctx.Done()
				return ctx.Err()
			}), nil
		})

		runner := NotifyOnSignal(
			DefaultRunner[Runtime](),
			syscall.SIGUSR1,
		)

		go func() {
			quiesce() // Wait for signal handler setup
			syscall.Kill(syscall.Getpid(), syscall.SIGUSR1)
		}()

		err := runner.Run(context.Background(), builder)
		require.ErrorIs(t, err, context.Canceled)
	})
}

func TestRecoverPanics(t *testing.T) {
	testCases := []struct {
		name        string
		builder     Builder[Runtime]
		expectErr   bool
		expectPanic bool
		errorSubstr string
	}{
		{
			name: "wraps runner successfully",
			builder: BuilderFunc[Runtime](func(ctx context.Context) (Runtime, error) {
				return RuntimeFunc(func(ctx context.Context) error {
					return nil
				}), nil
			}),
		},
		{
			name: "propagates runner error normally",
			builder: BuilderFunc[Runtime](func(ctx context.Context) (Runtime, error) {
				return RuntimeFunc(func(ctx context.Context) error {
					return errors.New("normal error")
				}), nil
			}),
			expectErr: true,
		},
		{
			name: "recovers from panic with string",
			builder: BuilderFunc[Runtime](func(ctx context.Context) (Runtime, error) {
				return RuntimeFunc(func(ctx context.Context) error {
					panic("test panic")
				}), nil
			}),
			expectErr: true,
			errorSubstr: "recovered from panic: test panic",
		},
		{
			name: "recovers from panic with error",
			builder: BuilderFunc[Runtime](func(ctx context.Context) (Runtime, error) {
				return RuntimeFunc(func(ctx context.Context) error {
					panic(errors.New("panic error"))
				}), nil
			}),
			expectErr: true,
			errorSubstr: "recovered from panic:",
		},
		{
			name: "recovers from panic with int",
			builder: BuilderFunc[Runtime](func(ctx context.Context) (Runtime, error) {
				return RuntimeFunc(func(ctx context.Context) error {
					panic(42)
				}), nil
			}),
			expectErr: true,
			errorSubstr: "recovered from panic: 42",
		},
		{
			name: "recovers from panic with nil",
			builder: BuilderFunc[Runtime](func(ctx context.Context) (Runtime, error) {
				return RuntimeFunc(func(ctx context.Context) error {
					panic(nil)
				}), nil
			}),
			expectErr: true,
			errorSubstr: "recovered from panic:",
		},
		{
			name: "passes context to wrapped runner",
			builder: BuilderFunc[Runtime](func(ctx context.Context) (Runtime, error) {
				if ctx == nil {
					return nil, errors.New("context is nil")
				}
				return RuntimeFunc(func(ctx context.Context) error {
					if ctx == nil {
						return errors.New("runtime context is nil")
					}
					return nil
				}), nil
			}),
		},
		{
			name: "returns error instead of panicking",
			builder: BuilderFunc[Runtime](func(ctx context.Context) (Runtime, error) {
				return RuntimeFunc(func(ctx context.Context) error {
					panic("should be caught")
				}), nil
			}),
			expectErr: true,
			errorSubstr: "recovered from panic: should be caught",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			runner := RecoverPanics(DefaultRunner[Runtime]())

			require.NotPanics(t, func() {
				err := runner.Run(context.Background(), tc.builder)

				if tc.expectErr {
					require.Error(t, err)
					if tc.errorSubstr != "" {
						require.Contains(t, err.Error(), tc.errorSubstr)
					}
				} else {
					require.NoError(t, err)
				}
			})
		})
	}
}
