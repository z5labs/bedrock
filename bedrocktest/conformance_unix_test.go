// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

//go:build unix

package bedrocktest_test

import (
	"context"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"testing"

	"github.com/z5labs/bedrock/bedrocktest"

	"github.com/stretchr/testify/require"
)

// notifier is one implementation of NotifyContext under test, with a way
// to deliver SIGUSR1 to it that returns only once the signal was delivered.
type notifier struct {
	notifyContext func(parent context.Context, signals ...os.Signal) (context.Context, context.CancelFunc)
	send          func(t *testing.T)
}

// TestNotifyContextConformance runs one table against the real
// signal.NotifyContext and against the fake, so that the fake cannot drift
// from the behaviour it stands in for.
//
// It sends real signals to the test binary, so it is deliberately not
// parallel.
func TestNotifyContextConformance(t *testing.T) {
	// The guard registration is held for the whole test. Even after a
	// NotifyContext's stop calls signal.Stop, SIGUSR1 then never falls back
	// to its default action, which would kill the test binary, and the
	// guard tells the real sender when a signal was delivered.
	guard := make(chan os.Signal, 1)
	signal.Notify(guard, syscall.SIGUSR1)
	t.Cleanup(func() { signal.Stop(guard) })

	notifiers := map[string]func(t *testing.T) notifier{
		"real": func(*testing.T) notifier {
			return notifier{
				notifyContext: signal.NotifyContext,
				send: func(t *testing.T) {
					require.NoError(t, syscall.Kill(os.Getpid(), syscall.SIGUSR1))
					<-guard
				},
			}
		},
		"fake": func(t *testing.T) notifier {
			fp := bedrocktest.NewProcess(t)
			// The fake's own guard keeps SIGUSR1 registered after stop,
			// as the real guard does, so a later Signal is delivered
			// rather than failing the test.
			_, stopGuard := fp.Process().NotifyContext(context.Background(), syscall.SIGUSR1)
			t.Cleanup(stopGuard)
			return notifier{
				notifyContext: fp.Process().NotifyContext,
				send:          func(*testing.T) { fp.Signal(syscall.SIGUSR1) },
			}
		},
	}

	testCases := []struct {
		name string
		run  func(t *testing.T, n notifier)
	}{
		{
			name: "derives from its parent",
			run: func(t *testing.T, n notifier) {
				type ctxKey struct{}
				parent, cancel := context.WithCancel(context.WithValue(context.Background(), ctxKey{}, "parent"))
				ctx, stop := n.notifyContext(parent, syscall.SIGUSR1)
				defer stop()

				require.Equal(t, "parent", ctx.Value(ctxKey{}))
				require.NoError(t, ctx.Err())

				cancel()
				<-ctx.Done()
				require.ErrorIs(t, ctx.Err(), context.Canceled)
				require.Equal(t, context.Canceled, context.Cause(ctx))
			},
		},
		{
			name: "delivery cancels with a cause naming the signal that matches context.Canceled",
			run: func(t *testing.T, n notifier) {
				ctx, stop := n.notifyContext(context.Background(), syscall.SIGUSR1)
				defer stop()
				require.NoError(t, ctx.Err())

				n.send(t)
				<-ctx.Done()

				require.ErrorIs(t, ctx.Err(), context.Canceled)
				cause := context.Cause(ctx)
				require.EqualError(t, cause, "user defined signal 1 signal received")
				require.ErrorIs(t, cause, context.Canceled)

				stop()
				require.EqualError(t, context.Cause(ctx), "user defined signal 1 signal received")
			},
		},
		{
			name: "listing no signals means every signal",
			run: func(t *testing.T, n notifier) {
				ctx, stop := n.notifyContext(context.Background())
				defer stop()

				n.send(t)
				<-ctx.Done()

				// The real NotifyContext also catches signals the runtime
				// sends itself, such as SIGURG for preemption, so the first
				// signal to arrive is not necessarily SIGUSR1.
				cause := context.Cause(ctx)
				require.True(t, strings.HasSuffix(cause.Error(), " signal received"), "cause: %v", cause)
				require.ErrorIs(t, cause, context.Canceled)
			},
		},
		{
			name: "stop cancels with context.Canceled and a later signal does not change it",
			run: func(t *testing.T, n notifier) {
				ctx, stop := n.notifyContext(context.Background(), syscall.SIGUSR1)
				stop()

				require.ErrorIs(t, ctx.Err(), context.Canceled)
				require.Equal(t, context.Canceled, context.Cause(ctx))

				n.send(t)
				require.Equal(t, context.Canceled, context.Cause(ctx))
			},
		},
	}

	for _, name := range []string{"real", "fake"} {
		t.Run(name, func(t *testing.T) {
			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					tc.run(t, notifiers[name](t))
				})
			}
		})
	}
}
