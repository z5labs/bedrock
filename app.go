// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package bedrock provides a minimal foundation for building more complex frameworks on top of.
package bedrock

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"github.com/z5labs/bedrock/pkg/config"
	"github.com/z5labs/bedrock/pkg/otelconfig"

	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel"
	"golang.org/x/sync/errgroup"
)

// Runtime
type Runtime interface {
	Run(context.Context) error
}

// Lifecycle provides the ability to hook into certain points of
// the bedrock App.Run process.
type Lifecycle struct {
	preRunHooks  []func(context.Context) error
	postRunHooks []func(context.Context) error
}

// PreRun registers hooks to be called before Runtime.Run is called.
func (l *Lifecycle) PreRun(hooks ...func(context.Context) error) {
	l.preRunHooks = append(l.preRunHooks, hooks...)
}

// PostRun registers hooks to be after Runtime.Run has completed, regardless
// whether it returned an error or not.
func (l *Lifecycle) PostRun(hooks ...func(context.Context) error) {
	l.postRunHooks = append(l.postRunHooks, hooks...)
}

// WithTracerProvider register lifecycle hooks for ensuring a global trace.TracerProvider is registered
// before Runtime.Run and that the TracerProvider is successfully shutdown after Runtime.Run.
func WithTracerProvider(life *Lifecycle, initer otelconfig.Initializer) {
	life.PreRun(func(ctx context.Context) error {
		tp, err := initer.Init()
		if err != nil {
			return err
		}
		otel.SetTracerProvider(tp)
		return nil
	})
	life.PostRun(finalizeOtel)
}

type contextKey string

var (
	configContextKey    = contextKey("configContextKey")
	lifecycleContextKey = contextKey("lifecycleContextKey")
)

// ConfigFromContext extracts a *config.Manager from the given context.Context if it's present.
func ConfigFromContext(ctx context.Context) *config.Manager {
	return ctx.Value(configContextKey).(*config.Manager)
}

// LifecycleFromContext extracts a *Lifecycle from the given context.Context if it's present.
func LifecycleFromContext(ctx context.Context) *Lifecycle {
	return ctx.Value(lifecycleContextKey).(*Lifecycle)
}

// RuntimeBuilder
type RuntimeBuilder interface {
	Build(context.Context) (Runtime, error)
}

// RuntimeBuilderFunc
type RuntimeBuilderFunc func(context.Context) (Runtime, error)

// Build implements the RuntimeBuilder interface.
func (f RuntimeBuilderFunc) Build(ctx context.Context) (Runtime, error) {
	return f(ctx)
}

// Option
type Option func(*App)

// Name
func Name(name string) Option {
	return func(a *App) {
		a.name = name
	}
}

// WithRuntimeBuilder
func WithRuntimeBuilder(rb RuntimeBuilder) Option {
	return func(a *App) {
		a.rbs = append(a.rbs, rb)
	}
}

// WithRuntimeBuilderFunc
func WithRuntimeBuilderFunc(f func(context.Context) (Runtime, error)) Option {
	return func(a *App) {
		a.rbs = append(a.rbs, RuntimeBuilderFunc(f))
	}
}

// Config
func Config(r io.Reader) Option {
	return func(a *App) {
		a.cfgSrc = r
	}
}

// App
type App struct {
	name           string
	cfgSrc         io.Reader
	otelIniterFunc func(context.Context) (otelconfig.Initializer, error)
	rbs            []RuntimeBuilder
}

// New
func New(opts ...Option) *App {
	var name string
	if len(os.Args) > 0 {
		name = os.Args[0]
	}
	app := &App{
		name: name,
		otelIniterFunc: func(_ context.Context) (otelconfig.Initializer, error) {
			return otelconfig.Noop, nil
		},
	}
	for _, opt := range opts {
		opt(app)
	}
	return app
}

// Run
func (app *App) Run(args ...string) error {
	cmd := buildCmd(app)
	cmd.SetArgs(args)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer cancel()

	return cmd.ExecuteContext(ctx)
}

var errNilRuntime = errors.New("nil runtime")

func buildCmd(app *App) *cobra.Command {
	var cfg config.Manager
	var life Lifecycle

	rs := make([]Runtime, len(app.rbs))

	return &cobra.Command{
		PreRunE: func(cmd *cobra.Command, args []string) (err error) {
			defer errRecover(&err)
			if app.cfgSrc != nil {
				b, err := readAllAndTryClose(app.cfgSrc)
				if err != nil {
					return err
				}

				m, err := config.Read(bytes.NewReader(b), config.Language(config.YAML))
				if err != nil {
					return err
				}
				cfg = m
			}

			ctx := context.WithValue(cmd.Context(), configContextKey, cfg)
			ctx = context.WithValue(ctx, lifecycleContextKey, &life)

			for i, rb := range app.rbs {
				r, err := rb.Build(ctx)
				if err != nil {
					return err
				}
				if r == nil {
					return errNilRuntime
				}
				rs[i] = r
			}

			var me multiError
			for _, f := range life.preRunHooks {
				err := f(ctx)
				if err != nil {
					me.errors = append(me.errors, err)
				}
			}
			if len(me.errors) == 0 {
				return nil
			}
			return me
		},
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			defer errRecover(&err)

			if len(rs) == 0 {
				return
			}
			if len(rs) == 1 {
				return rs[0].Run(cmd.Context())
			}

			g, gctx := errgroup.WithContext(cmd.Context())
			for _, rt := range rs {
				rt := rt
				g.Go(func() (e error) {
					defer errRecover(&e)
					return rt.Run(gctx)
				})
			}
			return g.Wait()
		},
		PostRunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.WithValue(cmd.Context(), configContextKey, cfg)
			ctx = context.WithValue(ctx, lifecycleContextKey, &life)

			var me multiError
			for _, f := range life.postRunHooks {
				err := f(ctx)
				if err != nil {
					me.errors = append(me.errors, err)
				}
			}

			if len(me.errors) == 0 {
				return nil
			}
			return me
		},
	}
}

type multiError struct {
	errors []error
}

func (m multiError) Error() string {
	if len(m.errors) == 0 {
		return ""
	}

	e := ""
	for _, err := range m.errors {
		e += err.Error() + ";"
	}

	return strings.TrimSuffix(e, ";")
}

func finalizeOtel(ctx context.Context) error {
	tp := otel.GetTracerProvider()
	stp, ok := tp.(interface {
		Shutdown(context.Context) error
	})
	if !ok {
		return nil
	}
	return stp.Shutdown(ctx)
}

func readAllAndTryClose(r io.Reader) ([]byte, error) {
	defer func() {
		rc, ok := r.(io.ReadCloser)
		if !ok {
			return
		}
		rc.Close()
	}()
	return io.ReadAll(r)
}

type panicError struct {
	v any
}

func (e panicError) Error() string {
	return fmt.Sprintf("bedrock: recovered from a panic caused by: %v", e.v)
}

func errRecover(err *error) {
	r := recover()
	if r == nil {
		return
	}
	rerr, ok := r.(error)
	if !ok {
		*err = panicError{v: r}
		return
	}
	*err = rerr
}
