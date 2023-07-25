// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package app

import (
	"context"

	"golang.org/x/sync/errgroup"
)

// MultiRuntime takes inspiration from the io.Multiwriter
// to allow users to run multiple app runtimes concurrently.
type MultiRuntime struct {
	rs []Runtime
}

// WithRuntimes
func WithRuntimes(rbs ...RuntimeBuilder) RuntimeBuilder {
	return RuntimeBuilderFunc(func(ctx BuildContext) (Runtime, error) {
		rs := make([]Runtime, len(rbs))
		for i, rb := range rbs {
			r, err := rb.Build(ctx)
			if err != nil {
				return nil, err
			}
			rs[i] = r
		}
		mr := &MultiRuntime{
			rs: rs,
		}
		return mr, nil
	})
}

// Run implements the Runtime interface.
func (mr *MultiRuntime) Run(ctx context.Context) error {
	g, gctx := errgroup.WithContext(ctx)
	for _, r := range mr.rs {
		r := r
		g.Go(func() error {
			return r.Run(gctx)
		})
	}
	return g.Wait()
}
