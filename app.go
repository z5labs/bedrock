// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/signal"

	"github.com/z5labs/app/pkg/config"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
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

func Config(r io.Reader) Option {
	return func(a *App) {
		a.cfgSrc = r
	}
}

// App
type App struct {
	name   string
	cfgSrc io.Reader
	rbs    []RuntimeBuilder
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
		PreRunE: func(cmd *cobra.Command, args []string) (err error) {
			defer errRecover(&err)

			b, err := readAllAndTryClose(app.cfgSrc)
			if err != nil {
				return err
			}

			m, err := config.Read(bytes.NewReader(b), config.Language(config.YAML))
			if err != nil {
				return err
			}

			for i, rb := range app.rbs {
				r, err := rb.Build(BuildContext{Config: m})
				if err != nil {
					return err
				}
				if r == nil {
					return errors.New("nil runtime")
				}
				rs[i] = r
			}
			return nil
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
	}
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
