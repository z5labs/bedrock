// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/z5labs/bedrock"
	"github.com/z5labs/bedrock/config"
)

// App is the HTTP service. Both of its fields are required.
//
// The network is the App's to inject, not bedrock's: bedrock only hands an
// App what exec hands a process, so the App takes the network as a field.
type App struct {
	// Listen opens the listener the service accepts connections on. main
	// sets it to net.Listen, and a test sets it to an in-memory listener.
	Listen func(network, addr string) (net.Listener, error)

	// Handler builds the handler the service serves, given the logger the
	// App built from its process. main sets it to NewHandler.
	Handler func(log *slog.Logger) http.Handler
}

// Settings is the configuration of the service, read from the environment
// of its process. See the package documentation for the variables and
// their defaults.
type Settings struct {
	Port                uint16
	ReadTimeout         time.Duration
	ReadHeaderTimeout   time.Duration
	WriteTimeout        time.Duration
	IdleTimeout         time.Duration
	MaxHeaderBytes      int
	ShutdownGracePeriod time.Duration
}

// readSettings reads the Settings from env. Every malformed variable is
// reported in the one error it returns, and none of them falls back to its
// default.
func readSettings(ctx context.Context, env bedrock.Environment) (Settings, error) {
	return config.Struct(func(r *config.Resolver) Settings {
		duration := func(name string, d time.Duration) config.Value[time.Duration] {
			return config.Env(env, name).Parse(time.ParseDuration).Default(d)
		}
		return Settings{
			Port:                r.Get(config.Env(env, "PORT").Parse(parsePort).Default(8080)),
			ReadTimeout:         r.Get(duration("READ_TIMEOUT", 5*time.Second)),
			ReadHeaderTimeout:   r.Get(duration("READ_HEADER_TIMEOUT", 2*time.Second)),
			WriteTimeout:        r.Get(duration("WRITE_TIMEOUT", 10*time.Second)),
			IdleTimeout:         r.Get(duration("IDLE_TIMEOUT", 120*time.Second)),
			MaxHeaderBytes:      r.Get(config.Env(env, "MAX_HEADER_BYTES").Parse(strconv.Atoi).Default(1048576)),
			ShutdownGracePeriod: r.Get(duration("SHUTDOWN_GRACE_PERIOD", 10*time.Second)),
		}
	}).Read(ctx)
}

// parsePort parses s as a TCP port, 0..65535.
func parsePort(s string) (uint16, error) {
	n, err := strconv.ParseUint(s, 10, 16)
	return uint16(n), err
}

// Run implements bedrock.App. It reads its settings, listens, and serves
// until an interrupt or SIGTERM, then shuts down gracefully.
//
// A failure is written to p.Stderr, since bedrock.Main prints nothing, and
// is also returned as the cause of the status for tests.
func (a App) Run(ctx context.Context, p bedrock.Process) bedrock.ExitStatus {
	settings, err := readSettings(ctx, p.Environment)
	if err != nil {
		return fail(p, err)
	}

	log := slog.New(slog.NewJSONHandler(p.Stdout, nil))

	ctx, stop := p.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	addr := net.JoinHostPort("", strconv.Itoa(int(settings.Port)))
	ln, err := a.Listen("tcp", addr)
	if err != nil {
		return fail(p, fmt.Errorf("listen: %w", err))
	}

	srv := &http.Server{
		Handler:           a.Handler(log),
		ReadTimeout:       settings.ReadTimeout,
		ReadHeaderTimeout: settings.ReadHeaderTimeout,
		WriteTimeout:      settings.WriteTimeout,
		IdleTimeout:       settings.IdleTimeout,
		MaxHeaderBytes:    settings.MaxHeaderBytes,
		// Without an ErrorLog, http.Server logs through the log package's
		// default logger, which writes to the real standard error.
		ErrorLog: slog.NewLogLogger(log.Handler(), slog.LevelError),
	}

	log.InfoContext(ctx, "serving", slog.String("addr", ln.Addr().String()))

	// serving is cancelled when a signal arrives or when Serve fails, and
	// either one starts the shutdown.
	serving, stopServing := context.WithCancelCause(ctx)
	defer stopServing(nil)

	var wg sync.WaitGroup
	var serveErr, shutdownErr error
	wg.Go(func() {
		serveErr = srv.Serve(ln)
		stopServing(fmt.Errorf("serve: %w", serveErr))
	})
	wg.Go(func() {
		<-serving.Done()
		log.InfoContext(ctx, "shutting down", slog.Any("cause", context.Cause(serving)))

		// The grace period starts now, from a context that the signal
		// that cancelled ctx does not cancel.
		graceCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), settings.ShutdownGracePeriod)
		defer cancel()
		shutdownErr = srv.Shutdown(graceCtx)
	})
	wg.Wait()

	var errs []error
	if !errors.Is(serveErr, http.ErrServerClosed) {
		errs = append(errs, fmt.Errorf("serve: %w", serveErr))
	}
	if shutdownErr != nil {
		// The grace period is up, so drop the connections still open.
		errs = append(errs, fmt.Errorf("shutdown: %w", shutdownErr), srv.Close())
	}
	if err := errors.Join(errs...); err != nil {
		return fail(p, err)
	}

	log.InfoContext(ctx, "stopped")
	return bedrock.ExitWithCause(0, context.Cause(ctx))
}

// fail writes err to the standard error of p and returns a status with
// exit code 1 and err as its cause.
func fail(p bedrock.Process, err error) bedrock.ExitStatus {
	if _, werr := fmt.Fprintln(p.Stderr, err); werr != nil {
		return bedrock.ExitWithCause(1, errors.Join(err, werr))
	}
	return bedrock.ExitWithCause(1, err)
}

// NewHandler returns the handler of the service, which answers
// GET /hello with "hello, world".
func NewHandler(log *slog.Logger) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /hello", func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.WriteString(w, "hello, world\n"); err != nil {
			log.ErrorContext(r.Context(), "write response", slog.Any("error", err))
		}
	})
	return mux
}
