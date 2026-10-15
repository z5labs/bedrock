// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"syscall"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/z5labs/bedrock"
	"github.com/z5labs/bedrock/bedrocktest"
)

// pipeListener is an in-memory net.Listener. Each Dial makes a net.Pipe and
// hands its server end to Accept, so no real socket is opened and the
// whole exchange can run inside a testing/synctest bubble.
type pipeListener struct {
	conns     chan net.Conn
	closed    chan struct{}
	closeOnce sync.Once
}

func newPipeListener() *pipeListener {
	return &pipeListener{
		conns:  make(chan net.Conn),
		closed: make(chan struct{}),
	}
}

func (l *pipeListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.conns:
		return c, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}

func (l *pipeListener) Close() error {
	l.closeOnce.Do(func() { close(l.closed) })
	return nil
}

func (l *pipeListener) Addr() net.Addr {
	return pipeAddr{}
}

// DialContext is the DialContext of an http.Transport that dials l.
func (l *pipeListener) DialContext(ctx context.Context, _, _ string) (net.Conn, error) {
	server, client := net.Pipe()
	select {
	case l.conns <- server:
		return client, nil
	case <-l.closed:
		return nil, errors.Join(net.ErrClosed, server.Close(), client.Close())
	case <-ctx.Done():
		return nil, errors.Join(ctx.Err(), server.Close(), client.Close())
	}
}

// client returns an http.Client whose every connection is dialled on l.
func (l *pipeListener) client() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext:       l.DialContext,
			DisableKeepAlives: true,
		},
	}
}

type pipeAddr struct{}

func (pipeAddr) Network() string { return "pipe" }
func (pipeAddr) String() string  { return "pipe" }

// listenCall records one call of App.Listen.
type listenCall struct {
	network, addr string
}

// listenOn returns an App.Listen that hands out ln and records each call
// in calls. calls must only be read once Run has returned.
func listenOn(ln net.Listener, calls *[]listenCall) func(network, addr string) (net.Listener, error) {
	return func(network, addr string) (net.Listener, error) {
		*calls = append(*calls, listenCall{network: network, addr: addr})
		return ln, nil
	}
}

type response struct {
	status int
	body   string
	err    error
}

// get sends GET url with c and reports the outcome on the returned channel.
func get(ctx context.Context, c *http.Client, url string) <-chan response {
	ch := make(chan response, 1)
	go func() {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			ch <- response{err: err}
			return
		}
		resp, err := c.Do(req)
		if err != nil {
			ch <- response{err: err}
			return
		}
		body, err := io.ReadAll(resp.Body)
		ch <- response{status: resp.StatusCode, body: string(body), err: errors.Join(err, resp.Body.Close())}
	}()
	return ch
}

// blockingHandler handles one request: it closes started when the request
// arrives, answers "done" once release is closed, and reports the outcome
// of that write on written, which must have room for it.
func blockingHandler(started chan<- struct{}, release <-chan struct{}, written chan<- error) func(*slog.Logger) http.Handler {
	return func(*slog.Logger) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			close(started)
			<-release
			_, err := io.WriteString(w, "done")
			written <- err
		})
	}
}

func TestReadSettings(t *testing.T) {
	t.Parallel()

	t.Run("defaults", func(t *testing.T) {
		t.Parallel()

		got, err := readSettings(t.Context(), bedrock.NewStaticEnvironment(nil))
		require.NoError(t, err)
		require.Equal(t, Settings{
			Port:                8080,
			ReadTimeout:         5 * time.Second,
			ReadHeaderTimeout:   2 * time.Second,
			WriteTimeout:        10 * time.Second,
			IdleTimeout:         120 * time.Second,
			MaxHeaderBytes:      1048576,
			ShutdownGracePeriod: 10 * time.Second,
		}, got)
	})

	t.Run("every variable set", func(t *testing.T) {
		t.Parallel()

		got, err := readSettings(t.Context(), bedrock.NewStaticEnvironment(map[string]string{
			"PORT":                  "9090",
			"READ_TIMEOUT":          "1s",
			"READ_HEADER_TIMEOUT":   "2s",
			"WRITE_TIMEOUT":         "3s",
			"IDLE_TIMEOUT":          "4s",
			"MAX_HEADER_BYTES":      "4096",
			"SHUTDOWN_GRACE_PERIOD": "5s",
		}))
		require.NoError(t, err)
		require.Equal(t, Settings{
			Port:                9090,
			ReadTimeout:         1 * time.Second,
			ReadHeaderTimeout:   2 * time.Second,
			WriteTimeout:        3 * time.Second,
			IdleTimeout:         4 * time.Second,
			MaxHeaderBytes:      4096,
			ShutdownGracePeriod: 5 * time.Second,
		}, got)
	})
}

func TestApp_Run_malformedSetting(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		env  map[string]string
	}{
		{name: "PORT", env: map[string]string{"PORT": "http"}},
		{name: "PORT", env: map[string]string{"PORT": "65536"}},
		{name: "READ_TIMEOUT", env: map[string]string{"READ_TIMEOUT": "30"}},
		{name: "READ_HEADER_TIMEOUT", env: map[string]string{"READ_HEADER_TIMEOUT": "2"}},
		{name: "WRITE_TIMEOUT", env: map[string]string{"WRITE_TIMEOUT": "ten seconds"}},
		{name: "IDLE_TIMEOUT", env: map[string]string{"IDLE_TIMEOUT": ""}},
		{name: "MAX_HEADER_BYTES", env: map[string]string{"MAX_HEADER_BYTES": "1MB"}},
		{name: "SHUTDOWN_GRACE_PERIOD", env: map[string]string{"SHUTDOWN_GRACE_PERIOD": "30"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name+"="+tc.env[tc.name], func(t *testing.T) {
			t.Parallel()

			var calls []listenCall
			app := App{Listen: listenOn(newPipeListener(), &calls), Handler: NewHandler}
			fp := bedrocktest.NewProcess(t, bedrocktest.WithEnv(tc.env))

			status := app.Run(t.Context(), fp.Process())

			require.NotZero(t, status.Code())
			require.ErrorContains(t, status.Cause(), "config "+tc.name+":")
			require.Contains(t, fp.Stderr(), "config "+tc.name+":")
			require.Empty(t, calls, "Listen must not be called when a setting is malformed")
		})
	}

	t.Run("two malformed variables are reported together", func(t *testing.T) {
		t.Parallel()

		var calls []listenCall
		app := App{Listen: listenOn(newPipeListener(), &calls), Handler: NewHandler}
		fp := bedrocktest.NewProcess(t, bedrocktest.WithEnv(map[string]string{
			"READ_TIMEOUT":     "30",
			"MAX_HEADER_BYTES": "lots",
		}))

		status := app.Run(t.Context(), fp.Process())

		require.NotZero(t, status.Code())
		cause := status.Cause()
		require.ErrorContains(t, cause, "config READ_TIMEOUT:")
		require.ErrorContains(t, cause, "config MAX_HEADER_BYTES:")
		require.Contains(t, fp.Stderr(), "config READ_TIMEOUT:")
		require.Contains(t, fp.Stderr(), "config MAX_HEADER_BYTES:")
		require.Empty(t, calls)
	})
}

// failingWriter is a standard stream whose every write fails.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("stderr is closed")
}

func TestApp_Run_stderrWriteFails(t *testing.T) {
	t.Parallel()

	var calls []listenCall
	app := App{Listen: listenOn(newPipeListener(), &calls), Handler: NewHandler}
	fp := bedrocktest.NewProcess(t, bedrocktest.WithEnv(map[string]string{"READ_TIMEOUT": "30"}))
	p := fp.Process()
	p.Stderr = failingWriter{}

	status := app.Run(t.Context(), p)

	require.Equal(t, 1, status.Code())
	require.ErrorContains(t, status.Cause(), "config READ_TIMEOUT:")
	require.ErrorContains(t, status.Cause(), "stderr is closed")
}

func TestApp_Run_listenFails(t *testing.T) {
	t.Parallel()

	app := App{
		Listen: func(network, addr string) (net.Listener, error) {
			return nil, errors.New("address already in use")
		},
		Handler: NewHandler,
	}
	fp := bedrocktest.NewProcess(t, bedrocktest.WithEnv(map[string]string{"PORT": "9090"}))

	status := app.Run(t.Context(), fp.Process())

	require.Equal(t, 1, status.Code())
	require.ErrorContains(t, status.Cause(), "listen: address already in use")
	require.Contains(t, fp.Stderr(), "listen: address already in use")
}

// brokenListener is a net.Listener whose Accept fails permanently.
type brokenListener struct{}

func (brokenListener) Accept() (net.Conn, error) { return nil, errors.New("accept failed") }
func (brokenListener) Close() error              { return nil }
func (brokenListener) Addr() net.Addr            { return pipeAddr{} }

func TestApp_Run_serveFails(t *testing.T) {
	t.Parallel()

	app := App{
		Listen: func(network, addr string) (net.Listener, error) {
			return brokenListener{}, nil
		},
		Handler: NewHandler,
	}
	fp := bedrocktest.NewProcess(t)

	status := app.Run(t.Context(), fp.Process())

	require.Equal(t, 1, status.Code())
	require.ErrorContains(t, status.Cause(), "serve: accept failed")
	require.Contains(t, fp.Stderr(), "serve: accept failed")
	require.Contains(t, fp.Stdout(), `"msg":"shutting down"`)
}

func TestApp_Run_servesUntilSIGTERM(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		ln := newPipeListener()
		var calls []listenCall
		app := App{Listen: listenOn(ln, &calls), Handler: NewHandler}
		fp := bedrocktest.NewProcess(t)

		statusCh := make(chan bedrock.ExitStatus, 1)
		go func() { statusCh <- app.Run(t.Context(), fp.Process()) }()

		resp := <-get(t.Context(), ln.client(), "http://example/hello")
		require.NoError(t, resp.err)
		require.Equal(t, http.StatusOK, resp.status)
		require.Equal(t, "hello, world\n", resp.body)

		<-fp.Notified(syscall.SIGTERM)
		fp.Signal(syscall.SIGTERM)
		status := <-statusCh

		require.Equal(t, 0, status.Code())
		require.ErrorContains(t, status.Cause(), "terminated signal received")
		require.ErrorIs(t, status.Cause(), context.Canceled)
		require.Equal(t, []listenCall{{network: "tcp", addr: ":8080"}}, calls)
		require.Contains(t, fp.Stdout(), `"msg":"serving"`)
		require.Contains(t, fp.Stdout(), `"msg":"shutting down","cause":"terminated signal received"`)
		require.Contains(t, fp.Stdout(), `"msg":"stopped"`)
		require.Empty(t, fp.Stderr())
	})
}

func TestApp_Run_inFlightRequestCompletesOnSIGTERM(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		ln := newPipeListener()
		var calls []listenCall
		started := make(chan struct{})
		release := make(chan struct{})
		written := make(chan error, 1)
		app := App{Listen: listenOn(ln, &calls), Handler: blockingHandler(started, release, written)}
		fp := bedrocktest.NewProcess(t)

		statusCh := make(chan bedrock.ExitStatus, 1)
		go func() { statusCh <- app.Run(t.Context(), fp.Process()) }()

		respCh := get(t.Context(), ln.client(), "http://example/")
		<-started

		<-fp.Notified(syscall.SIGTERM)
		fp.Signal(syscall.SIGTERM)

		// Every goroutine is now blocked: the App is waiting in Shutdown
		// for the request it is still handling.
		synctest.Wait()
		require.Contains(t, fp.Stdout(), `"msg":"shutting down","cause":"terminated signal received"`)
		select {
		case <-statusCh:
			require.FailNow(t, "Run returned before the in-flight request completed")
		default:
		}

		close(release)
		resp := <-respCh
		status := <-statusCh

		require.NoError(t, <-written)
		require.NoError(t, resp.err)
		require.Equal(t, http.StatusOK, resp.status)
		require.Equal(t, "done", resp.body)
		require.Equal(t, 0, status.Code())
		require.ErrorContains(t, status.Cause(), "terminated signal received")
		require.Contains(t, fp.Stdout(), `"msg":"stopped"`)
	})
}

func TestApp_Run_shutdownGracePeriodExceeded(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		const grace = 3 * time.Second

		ln := newPipeListener()
		var calls []listenCall
		started := make(chan struct{})
		release := make(chan struct{})
		written := make(chan error, 1)
		app := App{Listen: listenOn(ln, &calls), Handler: blockingHandler(started, release, written)}
		fp := bedrocktest.NewProcess(t, bedrocktest.WithEnv(map[string]string{
			"SHUTDOWN_GRACE_PERIOD": grace.String(),
		}))

		statusCh := make(chan bedrock.ExitStatus, 1)
		go func() { statusCh <- app.Run(t.Context(), fp.Process()) }()

		respCh := get(t.Context(), ln.client(), "http://example/")
		<-started

		<-fp.Notified(syscall.SIGTERM)
		signalled := time.Now()
		fp.Signal(syscall.SIGTERM)
		status := <-statusCh

		require.Equal(t, grace, time.Since(signalled))
		require.Equal(t, 1, status.Code())
		require.ErrorIs(t, status.Cause(), context.DeadlineExceeded)
		require.ErrorContains(t, status.Cause(), "shutdown:")
		require.Contains(t, fp.Stderr(), "shutdown: context deadline exceeded")

		// The App closed the connection when the grace period was up, so
		// the client never gets the response.
		close(release)
		resp := <-respCh
		require.Error(t, resp.err)
		// The handler's write lands in the response buffer and succeeds
		// whether or not the connection is open, so only the client can
		// tell that it was dropped.
		require.NoError(t, <-written)
	})
}

// failingResponseWriter is an http.ResponseWriter whose body writes fail.
type failingResponseWriter struct {
	*httptest.ResponseRecorder
}

func (failingResponseWriter) Write([]byte) (int, error) {
	return 0, errors.New("connection reset")
}

func (failingResponseWriter) WriteString(string) (int, error) {
	return 0, errors.New("connection reset")
}

func TestNewHandler(t *testing.T) {
	t.Parallel()

	t.Run("hello", func(t *testing.T) {
		t.Parallel()

		var logs strings.Builder
		h := NewHandler(slog.New(slog.NewJSONHandler(&logs, nil)))
		rec := httptest.NewRecorder()

		h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/hello", nil))

		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, "hello, world\n", rec.Body.String())
		require.Empty(t, logs.String())
	})

	t.Run("write fails", func(t *testing.T) {
		t.Parallel()

		var logs strings.Builder
		h := NewHandler(slog.New(slog.NewJSONHandler(&logs, nil)))

		h.ServeHTTP(
			failingResponseWriter{httptest.NewRecorder()},
			httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/hello", nil),
		)

		require.Contains(t, logs.String(), `"msg":"write response","error":"connection reset"`)
	})
}
