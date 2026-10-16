// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package bedrocktest_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"testing/synctest"
	"time"

	"github.com/z5labs/bedrock"
	"github.com/z5labs/bedrock/bedrocktest"

	"github.com/stretchr/testify/require"
)

// recordingTB is a testing.TB that records the failures and cleanups the
// fake registers with it instead of acting on them, so a test can assert
// that the fake fails a test that misuses it. Its Context is cancelled by
// runCleanups, just before the cleanups run, as the testing package does.
// Every other method goes to the real testing.TB it embeds.
type recordingTB struct {
	testing.TB

	ctx    context.Context
	cancel context.CancelFunc

	mu       sync.Mutex
	errors   []string
	cleanups []func()
}

func newRecordingTB(t *testing.T) *recordingTB {
	ctx, cancel := context.WithCancel(t.Context())
	return &recordingTB{TB: t, ctx: ctx, cancel: cancel}
}

func (r *recordingTB) Context() context.Context {
	return r.ctx
}

func (r *recordingTB) Errorf(format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.errors = append(r.errors, fmt.Sprintf(format, args...))
}

func (r *recordingTB) Cleanup(f func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cleanups = append(r.cleanups, f)
}

// runCleanups cancels the Context, then runs the recorded cleanups
// last-registered first, as the testing package does.
func (r *recordingTB) runCleanups() {
	r.cancel()

	r.mu.Lock()
	cleanups := r.cleanups
	r.cleanups = nil
	r.mu.Unlock()

	for i := len(cleanups) - 1; i >= 0; i-- {
		cleanups[i]()
	}
}

func (r *recordingTB) recorded() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.errors...)
}

func (r *recordingTB) registered() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.cleanups)
}

func requireClosed(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	default:
		t.Fatal("channel is not closed")
	}
}

func requireOpen(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
		t.Fatal("channel is closed")
	default:
	}
}

func TestNewProcess(t *testing.T) {
	t.Parallel()

	t.Run("sets every field of the Process", func(t *testing.T) {
		t.Parallel()

		// Ranging over every field, rather than naming them, is what makes
		// a field added to bedrock.Process later fail here until
		// bedrocktest fills it.
		p := reflect.ValueOf(bedrocktest.NewProcess(t).Process())
		require.Positive(t, p.NumField())
		for i := range p.NumField() {
			name := p.Type().Field(i).Name
			require.False(t, p.Field(i).IsZero(), "Process().%s is not set", name)
		}
	})

	t.Run("defaults to the program name alone, empty stdin and an empty environment", func(t *testing.T) {
		t.Parallel()

		p := bedrocktest.NewProcess(t).Process()
		require.Equal(t, []string{bedrocktest.ProgramName}, p.Args)

		n, err := p.Stdin.Read(make([]byte, 8))
		require.Zero(t, n)
		require.ErrorIs(t, err, io.EOF)

		for _, name := range []string{"PATH", "HOME", "PORT"} {
			_, err := p.Environment.Value(name)
			var notFound bedrock.ValueNotFoundError
			require.ErrorAs(t, err, &notFound)
			require.Equal(t, name, notFound.Name)
		}
	})

	t.Run("WithArgs sets the arguments after the program name", func(t *testing.T) {
		t.Parallel()

		args := []string{"-addr", ":0"}
		fp := bedrocktest.NewProcess(t, bedrocktest.WithArgs("ignored"), bedrocktest.WithArgs(args...))
		args[0] = "changed"

		require.Equal(t, []string{bedrocktest.ProgramName, "-addr", ":0"}, fp.Process().Args)
	})

	t.Run("Process returns a fresh copy of Args each time", func(t *testing.T) {
		t.Parallel()

		fp := bedrocktest.NewProcess(t, bedrocktest.WithArgs("a"))
		fp.Process().Args[1] = "changed"

		require.Equal(t, []string{bedrocktest.ProgramName, "a"}, fp.Process().Args)
	})

	t.Run("WithEnv copies the map it is given", func(t *testing.T) {
		t.Parallel()

		values := map[string]string{"PORT": "8080"}
		fp := bedrocktest.NewProcess(t, bedrocktest.WithEnv(values))
		values["PORT"] = "9090"
		values["ADDED"] = "added"

		env := fp.Process().Environment
		v, err := env.Value("PORT")
		require.NoError(t, err)
		require.Equal(t, "8080", v)

		_, err = env.Value("ADDED")
		var notFound bedrock.ValueNotFoundError
		require.ErrorAs(t, err, &notFound)
		require.Equal(t, "ADDED", notFound.Name)
	})

	t.Run("WithStdin sets the standard input", func(t *testing.T) {
		t.Parallel()

		fp := bedrocktest.NewProcess(t, bedrocktest.WithStdin(strings.NewReader("input")))

		b, err := io.ReadAll(fp.Process().Stdin)
		require.NoError(t, err)
		require.Equal(t, "input", string(b))
	})

	t.Run("WithStdin panics on a nil reader", func(t *testing.T) {
		t.Parallel()

		require.PanicsWithValue(t, "bedrocktest: WithStdin requires a non-nil io.Reader", func() {
			bedrocktest.WithStdin(nil)
		})
	})
}

func TestProcess_Stdio(t *testing.T) {
	t.Parallel()

	const (
		writers = 100
		writes  = 20
	)

	// Every write is one fixed-width line, so a snapshot is consistent
	// exactly when it is a whole number of well-formed lines.
	line := func(w, i int) string { return fmt.Sprintf("writer %03d write %03d\n", w, i) }
	lineLen := len(line(0, 0))

	requireConsistent := func(t *testing.T, snapshot string) {
		t.Helper()
		require.Zero(t, len(snapshot)%lineLen, "snapshot holds a partial write")
		for i := 0; i < len(snapshot); i += lineLen {
			l := snapshot[i : i+lineLen]
			if !strings.HasPrefix(l, "writer ") || strings.IndexByte(l, '\n') != lineLen-1 {
				require.Failf(t, "interleaved write", "%q", l)
			}
		}
	}

	for _, stream := range []struct {
		name     string
		writer   func(bedrock.Process) io.Writer
		snapshot func(*bedrocktest.Process) string
	}{
		{
			name:     "Stdout",
			writer:   func(p bedrock.Process) io.Writer { return p.Stdout },
			snapshot: (*bedrocktest.Process).Stdout,
		},
		{
			name:     "Stderr",
			writer:   func(p bedrock.Process) io.Writer { return p.Stderr },
			snapshot: (*bedrocktest.Process).Stderr,
		},
	} {
		t.Run(stream.name+" is safe for concurrent writes", func(t *testing.T) {
			t.Parallel()

			fp := bedrocktest.NewProcess(t)
			out := stream.writer(fp.Process())

			var wg sync.WaitGroup
			errs := make(chan error, writers*writes)
			for w := range writers {
				wg.Go(func() {
					for i := range writes {
						if _, err := io.WriteString(out, line(w, i)); err != nil {
							errs <- err
						}
					}
				})
			}

			done := make(chan struct{})
			go func() {
				wg.Wait()
				close(done)
			}()
		wait:
			for {
				requireConsistent(t, stream.snapshot(fp))
				select {
				case <-done:
					break wait
				default:
				}
			}
			close(errs)
			for err := range errs {
				require.NoError(t, err)
			}

			got := stream.snapshot(fp)
			requireConsistent(t, got)
			require.Len(t, got, writers*writes*lineLen, "bytes were lost")
			for w := range writers {
				for i := range writes {
					require.Contains(t, got, line(w, i))
				}
			}
		})
	}

	t.Run("Stdout and Stderr are separate", func(t *testing.T) {
		t.Parallel()

		fp := bedrocktest.NewProcess(t)
		p := fp.Process()
		_, err := io.WriteString(p.Stdout, "out")
		require.NoError(t, err)
		_, err = io.WriteString(p.Stderr, "err")
		require.NoError(t, err)

		require.Equal(t, "out", fp.Stdout())
		require.Equal(t, "err", fp.Stderr())
	})
}

func TestProcess_NotifyContext(t *testing.T) {
	t.Parallel()

	t.Run("derives from its parent", func(t *testing.T) {
		t.Parallel()

		type ctxKey struct{}
		errParent := errors.New("parent cancelled")
		parent, cancel := context.WithCancelCause(context.WithValue(t.Context(), ctxKey{}, "parent"))

		fp := bedrocktest.NewProcess(t)
		ctx, stop := fp.Process().NotifyContext(parent, syscall.SIGTERM)
		defer stop()

		require.Equal(t, "parent", ctx.Value(ctxKey{}))
		require.NoError(t, ctx.Err())

		cancel(errParent)
		<-ctx.Done()
		require.ErrorIs(t, context.Cause(ctx), errParent)
	})

	t.Run("a parent that is already done gives a done context", func(t *testing.T) {
		t.Parallel()

		parent, cancel := context.WithCancel(t.Context())
		cancel()

		fp := bedrocktest.NewProcess(t)
		ctx, stop := fp.Process().NotifyContext(parent, syscall.SIGTERM)
		defer stop()

		requireClosed(t, ctx.Done())
	})

	t.Run("Signal cancels with a cause naming the signal that matches context.Canceled", func(t *testing.T) {
		t.Parallel()

		fp := bedrocktest.NewProcess(t)
		ctx, stop := fp.Process().NotifyContext(t.Context(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()

		fp.Signal(syscall.SIGTERM)

		requireClosed(t, ctx.Done())
		require.ErrorIs(t, ctx.Err(), context.Canceled)
		cause := context.Cause(ctx)
		require.EqualError(t, cause, "terminated signal received")
		require.ErrorIs(t, cause, context.Canceled)
		require.NotErrorIs(t, cause, context.DeadlineExceeded)
	})

	t.Run("listing no signals means every signal", func(t *testing.T) {
		t.Parallel()

		fp := bedrocktest.NewProcess(t)
		ctx, stop := fp.Process().NotifyContext(t.Context())
		defer stop()

		fp.Signal(syscall.SIGQUIT)

		require.EqualError(t, context.Cause(ctx), "quit signal received")
	})

	t.Run("delivers only to registrations that want the signal", func(t *testing.T) {
		t.Parallel()

		fp := bedrocktest.NewProcess(t)
		term, stopTerm := fp.Process().NotifyContext(t.Context(), syscall.SIGTERM)
		defer stopTerm()
		hup, stopHup := fp.Process().NotifyContext(t.Context(), syscall.SIGHUP)
		defer stopHup()
		every, stopEvery := fp.Process().NotifyContext(t.Context())
		defer stopEvery()

		fp.Signal(syscall.SIGTERM)

		requireClosed(t, term.Done())
		requireClosed(t, every.Done())
		requireOpen(t, hup.Done())
	})

	t.Run("a delivered registration stays live until stopped", func(t *testing.T) {
		t.Parallel()

		rec := newRecordingTB(t)
		fp := bedrocktest.NewProcess(rec)
		ctx, stop := fp.Process().NotifyContext(t.Context(), syscall.SIGTERM)

		fp.Signal(syscall.SIGTERM)
		fp.Signal(syscall.SIGTERM)
		stop()
		rec.runCleanups()

		require.Empty(t, rec.recorded())
		require.EqualError(t, context.Cause(ctx), "terminated signal received")
	})

	t.Run("stop cancels with context.Canceled and a later Signal does not change it", func(t *testing.T) {
		t.Parallel()

		fp := bedrocktest.NewProcess(t)
		// A second registration keeps SIGTERM live after stop, as the
		// guard registration does in the real-signal conformance test.
		_, stopGuard := fp.Process().NotifyContext(t.Context(), syscall.SIGTERM)
		defer stopGuard()

		ctx, stop := fp.Process().NotifyContext(t.Context(), syscall.SIGTERM)
		stop()
		stop()

		require.ErrorIs(t, ctx.Err(), context.Canceled)
		require.Equal(t, context.Canceled, context.Cause(ctx))

		fp.Signal(syscall.SIGTERM)
		require.Equal(t, context.Canceled, context.Cause(ctx))
	})
}

func TestProcess_Strictness(t *testing.T) {
	t.Parallel()

	t.Run("Signal with no registration fails the test", func(t *testing.T) {
		t.Parallel()

		rec := newRecordingTB(t)
		fp := bedrocktest.NewProcess(rec)

		fp.Signal(syscall.SIGTERM)

		require.Equal(t, []string{
			"bedrocktest: Signal(terminated) with no live NotifyContext registered for it; a real process would have taken the signal's default action",
		}, rec.recorded())
	})

	t.Run("Signal with only registrations for other signals fails the test", func(t *testing.T) {
		t.Parallel()

		rec := newRecordingTB(t)
		fp := bedrocktest.NewProcess(rec)
		ctx, stop := fp.Process().NotifyContext(t.Context(), syscall.SIGHUP)
		defer stop()

		fp.Signal(syscall.SIGTERM)

		require.Len(t, rec.recorded(), 1)
		require.Contains(t, rec.recorded()[0], "Signal(terminated)")
		requireOpen(t, ctx.Done())
	})

	t.Run("Signal after every registration is stopped fails the test", func(t *testing.T) {
		t.Parallel()

		rec := newRecordingTB(t)
		fp := bedrocktest.NewProcess(rec)
		_, stop := fp.Process().NotifyContext(t.Context(), syscall.SIGTERM)
		stop()

		fp.Signal(syscall.SIGTERM)

		require.Len(t, rec.recorded(), 1)
		require.Contains(t, rec.recorded()[0], "Signal(terminated)")
	})

	t.Run("a NotifyContext that is never stopped fails the test in Cleanup", func(t *testing.T) {
		t.Parallel()

		rec := newRecordingTB(t)
		fp := bedrocktest.NewProcess(rec)
		listed, _ := fp.Process().NotifyContext(t.Context(), syscall.SIGINT, syscall.SIGTERM) //nolint:govet // the leak is the point
		every, _ := fp.Process().NotifyContext(t.Context())                                   //nolint:govet // the leak is the point
		_, stop := fp.Process().NotifyContext(t.Context(), syscall.SIGHUP)
		stop()

		require.Empty(t, rec.recorded())
		rec.runCleanups()

		require.Equal(t, []string{
			"bedrocktest: a NotifyContext for [interrupt terminated] was never stopped; call its stop function, typically with defer",
			"bedrocktest: a NotifyContext for every signal was never stopped; call its stop function, typically with defer",
		}, rec.recorded())
		requireClosed(t, listed.Done())
		requireClosed(t, every.Done())
	})

	t.Run("Notified is closed once a matching registration exists", func(t *testing.T) {
		t.Parallel()

		fp := bedrocktest.NewProcess(t)
		before := fp.Notified(syscall.SIGTERM)
		hupBefore := fp.Notified(syscall.SIGHUP)
		requireOpen(t, before)
		require.Equal(t, before, fp.Notified(syscall.SIGTERM), "Notified returned a second channel")

		_, stop := fp.Process().NotifyContext(t.Context(), syscall.SIGTERM)
		defer stop()

		requireClosed(t, before)
		requireClosed(t, fp.Notified(syscall.SIGTERM))
		requireOpen(t, hupBefore)
	})

	t.Run("Notified is closed by a registration for every signal", func(t *testing.T) {
		t.Parallel()

		fp := bedrocktest.NewProcess(t)
		before := fp.Notified(syscall.SIGTERM)

		_, stop := fp.Process().NotifyContext(t.Context())
		defer stop()

		requireClosed(t, before)
		requireClosed(t, fp.Notified(syscall.SIGHUP))
	})

	t.Run("Notified after every registration is stopped waits for the next one", func(t *testing.T) {
		t.Parallel()

		fp := bedrocktest.NewProcess(t)
		_, stopFirst := fp.Process().NotifyContext(t.Context(), syscall.SIGTERM)
		_, stopSecond := fp.Process().NotifyContext(t.Context(), syscall.SIGTERM)
		first := fp.Notified(syscall.SIGTERM)
		requireClosed(t, first)

		stopFirst()
		requireClosed(t, fp.Notified(syscall.SIGTERM))

		stopSecond()
		requireClosed(t, first)
		after := fp.Notified(syscall.SIGTERM)
		requireOpen(t, after)

		_, stopThird := fp.Process().NotifyContext(t.Context(), syscall.SIGTERM)
		defer stopThird()
		requireClosed(t, after)
	})
}

func TestProcess_Synctest(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		// The App shuts down gracefully, taking 30s of fake time after
		// SIGTERM before it exits.
		app := bedrock.AppFunc(func(ctx context.Context, p bedrock.Process) bedrock.ExitStatus {
			ctx, stop := p.NotifyContext(ctx, syscall.SIGTERM)
			defer stop()

			if _, err := io.WriteString(p.Stdout, "serving\n"); err != nil {
				return bedrock.ExitWithCause(1, err)
			}
			<-ctx.Done()
			<-time.After(30 * time.Second)
			if _, err := io.WriteString(p.Stdout, "stopped\n"); err != nil {
				return bedrock.ExitWithCause(1, err)
			}
			return bedrock.ExitWithCause(0, context.Cause(ctx))
		})

		fp, statusCh := bedrocktest.Start(t, app)

		<-fp.Notified(syscall.SIGTERM)
		start := time.Now()
		fp.Signal(syscall.SIGTERM)

		status := <-statusCh
		require.Equal(t, 30*time.Second, time.Since(start))
		require.Zero(t, status.Code())
		require.EqualError(t, status.Cause(), "terminated signal received")
		require.Equal(t, "serving\nstopped\n", fp.Stdout())

		// synctest.Test itself fails if any goroutine of the bubble is
		// still running when this function returns; Wait makes the point
		// here, before the assertions above are taken as the whole story.
		synctest.Wait()
	})
}

func TestStart(t *testing.T) {
	t.Parallel()

	t.Run("panics on a nil App before registering a cleanup", func(t *testing.T) {
		t.Parallel()

		rec := newRecordingTB(t)
		require.PanicsWithValue(t, "bedrocktest: Start requires a non-nil bedrock.App", func() {
			bedrocktest.Start(rec, nil)
		})
		require.Zero(t, rec.registered())
	})

	t.Run("delivers the code and cause the App returns", func(t *testing.T) {
		t.Parallel()

		errBoom := errors.New("boom")
		_, status := bedrocktest.Start(t, bedrock.AppFunc(func(context.Context, bedrock.Process) bedrock.ExitStatus {
			return bedrock.ExitWithCause(3, errBoom)
		}))

		st := <-status
		require.Equal(t, 3, st.Code())
		require.ErrorIs(t, st.Cause(), errBoom)
	})

	t.Run("runs the App on a Process built from its options", func(t *testing.T) {
		t.Parallel()

		app := bedrock.AppFunc(func(_ context.Context, p bedrock.Process) bedrock.ExitStatus {
			greeting, err := p.Environment.Value("GREETING")
			if err != nil {
				return bedrock.ExitWithCause(1, err)
			}
			name, err := io.ReadAll(p.Stdin)
			if err != nil {
				return bedrock.ExitWithCause(1, err)
			}
			if _, err := fmt.Fprintf(p.Stdout, "%s %s, %s\n", strings.Join(p.Args, " "), greeting, name); err != nil {
				return bedrock.ExitWithCause(1, err)
			}
			return bedrock.Exit(0)
		})

		fp, status := bedrocktest.Start(t, app,
			bedrocktest.WithArgs("-v", "x"),
			bedrocktest.WithEnv(map[string]string{"GREETING": "hi"}),
			bedrocktest.WithStdin(strings.NewReader("gopher")),
		)

		st := <-status
		require.Zero(t, st.Code())
		require.NoError(t, st.Cause())
		require.Equal(t, bedrocktest.ProgramName+" -v x hi, gopher\n", fp.Stdout())
	})

	t.Run("runs the App on the test's context and waits for it when the test ends", func(t *testing.T) {
		t.Parallel()

		returned := make(chan error, 1)
		t.Run("subtest", func(t *testing.T) {
			bedrocktest.Start(t, bedrock.AppFunc(func(ctx context.Context, _ bedrock.Process) bedrock.ExitStatus {
				<-ctx.Done()
				returned <- ctx.Err()
				return bedrock.Exit(0)
			}))
		})

		select {
		case err := <-returned:
			require.ErrorIs(t, err, context.Canceled)
		default:
			require.FailNow(t, "the App was still running when t.Run returned")
		}
	})

	t.Run("waits for the App before checking for unstopped NotifyContexts", func(t *testing.T) {
		t.Parallel()

		rec := newRecordingTB(t)
		app := bedrock.AppFunc(func(ctx context.Context, p bedrock.Process) bedrock.ExitStatus {
			ctx, stop := p.NotifyContext(ctx, syscall.SIGTERM)
			defer stop()

			<-ctx.Done()
			return bedrock.ExitWithCause(0, context.Cause(ctx))
		})
		fp, status := bedrocktest.Start(rec, app)
		<-fp.Notified(syscall.SIGTERM)

		rec.runCleanups()

		require.Empty(t, rec.recorded())
		select {
		case st := <-status:
			require.Zero(t, st.Code())
			require.ErrorIs(t, st.Cause(), context.Canceled)
		default:
			require.FailNow(t, "the App was still running after the cleanups ran")
		}
	})

	t.Run("a test that never receives the status neither blocks nor leaks the App", func(t *testing.T) {
		t.Parallel()

		var returned atomic.Int32
		// synctest.Test fails if a goroutine of the bubble outlives the
		// function, so the Apps' goroutines must have returned by then.
		synctest.Test(t, func(t *testing.T) {
			bedrocktest.Start(t, bedrock.AppFunc(func(context.Context, bedrock.Process) bedrock.ExitStatus {
				defer returned.Add(1)
				return bedrock.Exit(1)
			}))
			bedrocktest.Start(t, bedrock.AppFunc(func(ctx context.Context, _ bedrock.Process) bedrock.ExitStatus {
				defer returned.Add(1)
				<-ctx.Done()
				return bedrock.Exit(2)
			}))
		})
		require.Equal(t, int32(2), returned.Load())
	})
}
