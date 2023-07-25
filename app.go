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

// App
type App struct {
	name string
	rb   RuntimeBuilder
}

// New
func New(rb RuntimeBuilder, opts ...Option) *App {
	var name string
	if len(os.Args) > 0 {
		name = os.Args[0]
	}
	app := &App{
		name: name,
		rb:   rb,
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
	return &cobra.Command{
		Use: "",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			defer errRecover(&err)

			r, err := app.rb.Build(BuildContext{})
			if err != nil {
				return err
			}
			return r.Run(cmd.Context())
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
