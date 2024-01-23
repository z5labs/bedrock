// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

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

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

// Runtime represents the entry point for user specific code.
// The Runtime should not worry about things like OS interrupts
// and config parsing because App is responsible for managing those
// more "low-level" things. A Runtime should be purely focused on
// running use case specific code e.g. RESTful API, gRPC API, K8s Job, etc.
type Runtime interface {
	Run(context.Context) error
}

// Lifecycle provides the ability to hook into certain points of
// the bedrock App.Run process.
type Lifecycle struct {
	preRunHooks  []func(context.Context) error
	postRunHooks []func(context.Context) error
}

// PreRun registers hooks to be called after the config is parsed and before Runtime.Run is called.
func (l *Lifecycle) PreRun(hooks ...func(context.Context) error) {
	l.preRunHooks = append(l.preRunHooks, hooks...)
}

// PostRun registers hooks to be after Runtime.Run has completed, regardless
// whether it returned an error or not.
func (l *Lifecycle) PostRun(hooks ...func(context.Context) error) {
	l.postRunHooks = append(l.postRunHooks, hooks...)
}

type contextKey string

var (
	configContextKey    = contextKey("configContextKey")
	lifecycleContextKey = contextKey("lifecycleContextKey")
)

// ConfigFromContext extracts a config.Manager from the given context.Context if it's present.
func ConfigFromContext(ctx context.Context) config.Manager {
	return ctx.Value(configContextKey).(config.Manager)
}

// LifecycleFromContext extracts a *Lifecycle from the given context.Context if it's present.
func LifecycleFromContext(ctx context.Context) *Lifecycle {
	return ctx.Value(lifecycleContextKey).(*Lifecycle)
}

// RuntimeBuilder represents anything which can initialize a Runtime.
type RuntimeBuilder interface {
	Build(context.Context) (Runtime, error)
}

// RuntimeBuilderFunc is a functional implementation of
// the RuntimeBuilder interface.
type RuntimeBuilderFunc func(context.Context) (Runtime, error)

// Build implements the RuntimeBuilder interface.
func (f RuntimeBuilderFunc) Build(ctx context.Context) (Runtime, error) {
	return f(ctx)
}

// Option are used to configure an App.
type Option func(*App)

// Name configures the name of the application.
func Name(name string) Option {
	return func(a *App) {
		a.name = name
	}
}

// WithRuntimeBuilder registers the given RuntimeBuilder with the App.
func WithRuntimeBuilder(rb RuntimeBuilder) Option {
	return func(a *App) {
		a.rbs = append(a.rbs, rb)
	}
}

// WithRuntimeBuilderFunc registers the given function as a RuntimeBuilder.
func WithRuntimeBuilderFunc(f func(context.Context) (Runtime, error)) Option {
	return func(a *App) {
		a.rbs = append(a.rbs, RuntimeBuilderFunc(f))
	}
}

// Config registers a config source with the application.
// If used multiple times, subsequent configs will be merged
// with the very first Config provided. The subsequent configs
// values will override any previous configs values.
func Config(r io.Reader) Option {
	return func(a *App) {
		a.cfgSrcs = append(a.cfgSrcs, r)
	}
}

// Hooks allows you to register multiple lifecycle hooks.
func Hooks(fs ...func(*Lifecycle)) Option {
	return func(a *App) {
		for _, f := range fs {
			f(&a.life)
		}
	}
}

// App handles the lower level things of running a service in Go.
// App is responsible for the following:
//   - Parsing (and merging) you config(s)
//   - Calling your lifecycle hooks at the appropriate times
//   - Running your Runtime(s) and propogating any OS interrupts
//     via context.Context cancellation
type App struct {
	name    string
	cfgSrcs []io.Reader
	rbs     []RuntimeBuilder
	life    Lifecycle
}

// New returns a fully initialized App.
func New(opts ...Option) *App {
	var name string
	if len(os.Args) > 0 {
		name = os.Args[0]
	}
	app := &App{
		name: name,
	}
	for _, opt := range opts {
		opt(app)
	}
	return app
}

// Run executes the application. It also handles listening
// for interrupts from the underlying OS and terminates
// the application when one is received.
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

	rs := make([]Runtime, len(app.rbs))

	return &cobra.Command{
		PreRunE: func(cmd *cobra.Command, args []string) (err error) {
			defer errRecover(&err)
			for i, cfgSrc := range app.cfgSrcs {
				b, err := readAllAndTryClose(cfgSrc)
				if err != nil {
					return err
				}

				cfg, err = config.Merge(cfg, bytes.NewReader(b), config.Language(config.YAML))
				if err != nil {
					return err
				}

				// tell the garbage collector that we no longer
				// need that config source and it can be collected
				app.cfgSrcs[i] = nil
			}
			// we no longer need this slice since all configs have been merged
			app.cfgSrcs = nil

			ctx := context.WithValue(cmd.Context(), configContextKey, cfg)
			ctx = context.WithValue(ctx, lifecycleContextKey, &app.life)

			for i, rb := range app.rbs {
				r, err := rb.Build(ctx)
				if err != nil {
					return err
				}
				if r == nil {
					return errNilRuntime
				}
				rs[i] = r

				// tell the garbage collector that we no longer
				// need that runtime builder and it can be collected
				app.rbs[i] = nil
			}
			// we no longer need this slice since all runtime have been built
			app.rbs = nil

			var me multiError
			for _, f := range app.life.preRunHooks {
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
			ctx = context.WithValue(ctx, lifecycleContextKey, &app.life)

			var me multiError
			for _, f := range app.life.postRunHooks {
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
