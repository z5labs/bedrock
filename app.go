// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package app

import (
	"context"
	"os"
	"os/signal"

	"github.com/z5labs/app/config"
	"golang.org/x/sync/errgroup"

	"github.com/spf13/cobra"
)

// Runtime
type Runtime interface {
	Run(context.Context) error
}

// BuildContext
type BuildContext struct {
	Config config.Manager
}

// RuntimeBuilder
type RuntimeBuilder interface {
	Build(BuildContext) (Runtime, error)
}

// RuntimeBuilderFunc
type RuntimeBuilderFunc func(BuildContext) (Runtime, error)

// Build implements the RuntimeBuilder interface.
func (f RuntimeBuilderFunc) Build(ctx BuildContext) (Runtime, error) {
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

// WithRuntime
func WithRuntime(rb RuntimeBuilder) Option {
	return func(a *App) {
		a.rbs = append(a.rbs, rb)
	}
}

func WithRuntimeBuilderFunc(f func(BuildContext) (Runtime, error)) Option {
	return func(a *App) {
		a.rbs = append(a.rbs, RuntimeBuilderFunc(f))
	}
}

// App
type App struct {
	name string
	rbs  []RuntimeBuilder
}

// New
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

// Run
func (app *App) Run(args ...string) error {
	cmd := buildCmd(app)
	cmd.SetArgs(args)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer cancel()

	return cmd.ExecuteContext(ctx)
}

func buildCmd(app *App) *cobra.Command {
	rs := make([]Runtime, len(app.rbs))
	return &cobra.Command{
		PreRunE: func(cmd *cobra.Command, args []string) error {
			for i, rb := range app.rbs {
				r, err := rb.Build(BuildContext{})
				if err != nil {
					return err
				}
				rs[i] = r
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			defer errRecover(&err)

			if len(app.rbs) == 0 {
				return
			}
			if len(app.rbs) == 1 {
				rb := app.rbs[0]
				r, err := rb.Build(BuildContext{})
				if err != nil {
					return err
				}
				return r.Run(cmd.Context())
			}

			g, gctx := errgroup.WithContext(cmd.Context())
			for _, rb := range app.rbs {
				rb := rb
				g.Go(func() error {
					r, err := rb.Build(BuildContext{})
					if err != nil {
						return err
					}
					return r.Run(gctx)
				})
			}
			return g.Wait()
		},
	}
}

func errRecover(err *error) {
	r := recover()
	if r == nil {
		return
	}
	rerr, ok := r.(error)
	if !ok {
		return
	}
	*err = rerr
}
