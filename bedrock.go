// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package bedrock

import (
	"context"
)

// App represents the entry point for user specific code.
type App interface {
	Run(context.Context) error
}

// AppBuilder represents anything which can initialize a Runtime.
type AppBuilder[T any] interface {
	Build(ctx context.Context, cfg T) (App, error)
}

// AppBuilderFunc is a functional implementation of
// the AppBuilder interface.
type AppBuilderFunc[T any] func(context.Context, T) (App, error)

// Build implements the RuntimeBuilder interface.
func (f AppBuilderFunc[T]) Build(ctx context.Context, cfg T) (App, error) {
	return f(ctx, cfg)
}
