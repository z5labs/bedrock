// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package bedrock_test

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/z5labs/bedrock"

	"github.com/stretchr/testify/require"
)

var _ bedrock.App = bedrock.AppFunc(nil)

// newProcess returns a hand-built Process with every field set: no
// arguments beyond the program name, empty stdin, in-memory stdout and
// stderr, a StaticEnvironment holding env, and a NotifyContext that only
// derives from its parent.
func newProcess(env map[string]string) (p bedrock.Process, stdout, stderr *bytes.Buffer) {
	stdout = new(bytes.Buffer)
	stderr = new(bytes.Buffer)
	p = bedrock.Process{
		Args:        []string{"app"},
		Stdin:       strings.NewReader(""),
		Stdout:      stdout,
		Stderr:      stderr,
		Environment: bedrock.NewStaticEnvironment(env),
		NotifyContext: func(parent context.Context, _ ...os.Signal) (context.Context, context.CancelFunc) {
			return context.WithCancel(parent)
		},
	}
	return p, stdout, stderr
}

func TestAppFunc(t *testing.T) {
	t.Parallel()

	t.Run("runs the function on the context and process it is given", func(t *testing.T) {
		t.Parallel()

		type ctxKey struct{}
		ctx := context.WithValue(t.Context(), ctxKey{}, "root")
		p, stdout, stderr := newProcess(map[string]string{"GREETING": "hello"})
		p.Args = []string{"app", "world"}

		app := bedrock.AppFunc(func(ctx context.Context, p bedrock.Process) bedrock.ExitStatus {
			greeting, err := p.Environment.Value("GREETING")
			if err != nil {
				return bedrock.ExitWithCause(1, err)
			}
			if _, err := p.Stdout.Write([]byte(greeting + ", " + p.Args[1] + " from " + ctx.Value(ctxKey{}).(string))); err != nil {
				return bedrock.ExitWithCause(1, err)
			}
			return bedrock.Exit(3)
		})

		status := app.Run(ctx, p)
		require.Equal(t, 3, status.Code())
		require.NoError(t, status.Cause())
		require.Equal(t, "hello, world from root", stdout.String())
		require.Empty(t, stderr.String())
	})

	t.Run("cancelling the root context reaches a context derived through NotifyContext", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		p, _, _ := newProcess(nil)

		derived := make(chan struct{})
		app := bedrock.AppFunc(func(ctx context.Context, p bedrock.Process) bedrock.ExitStatus {
			ctx, stop := p.NotifyContext(ctx, os.Interrupt)
			defer stop()

			close(derived)
			<-ctx.Done()
			return bedrock.ExitWithCause(1, context.Cause(ctx))
		})

		statusCh := make(chan bedrock.ExitStatus, 1)
		go func() {
			statusCh <- app.Run(ctx, p)
		}()

		<-derived
		cancel()

		status := <-statusCh
		require.Equal(t, 1, status.Code())
		require.ErrorIs(t, status.Cause(), context.Canceled)
	})
}

func TestStaticEnvironment(t *testing.T) {
	t.Parallel()

	t.Run("returns the value of a variable that is set", func(t *testing.T) {
		t.Parallel()

		env := bedrock.NewStaticEnvironment(map[string]string{"NAME": "value"})

		v, err := env.Value("NAME")
		require.NoError(t, err)
		require.Equal(t, "value", v)
	})

	t.Run("is unaffected by later changes to the map it was built from", func(t *testing.T) {
		t.Parallel()

		values := map[string]string{"NAME": "before"}
		env := bedrock.NewStaticEnvironment(values)

		values["NAME"] = "after"
		values["ADDED"] = "added"

		v, err := env.Value("NAME")
		require.NoError(t, err)
		require.Equal(t, "before", v)

		_, err = env.Value("ADDED")
		var notFound bedrock.ValueNotFoundError
		require.ErrorAs(t, err, &notFound)
		require.Equal(t, "ADDED", notFound.Name)
	})

	t.Run("returns a ValueNotFoundError naming a variable that is not set", func(t *testing.T) {
		t.Parallel()

		for _, env := range []bedrock.StaticEnvironment{
			bedrock.NewStaticEnvironment(map[string]string{"OTHER": "value"}),
			bedrock.NewStaticEnvironment(nil),
			{},
		} {
			_, err := env.Value("MISSING")

			var notFound bedrock.ValueNotFoundError
			require.ErrorAs(t, err, &notFound)
			require.Equal(t, "MISSING", notFound.Name)
			require.ErrorContains(t, err, "MISSING")
		}
	})
}

func TestValueNotFoundError(t *testing.T) {
	t.Parallel()

	err := bedrock.ValueNotFoundError{Name: "NAME"}
	require.Equal(t, "environment variable not found: NAME", err.Error())
}
