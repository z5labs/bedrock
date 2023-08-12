// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package http

import (
	"context"
	"net"
	"net/http"

	"golang.org/x/sync/errgroup"
)

type runtimeOptions struct{}

type RuntimeOption func(*runtimeOptions)

type Runtime struct {
	h http.Handler
}

func NewRuntime(h http.Handler, opts ...RuntimeOption) *Runtime {
	return &Runtime{
		h: h,
	}
}

func (rt *Runtime) Run(ctx context.Context) error {
	ls, err := net.Listen("http", ":8080")
	if err != nil {
		return err
	}

	srv := &http.Server{
		Handler: rt.h,
	}

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		<-gctx.Done()
		return srv.Shutdown(context.Background())
	})
	g.Go(func() error {
		return srv.Serve(ls)
	})
	return g.Wait()
}
