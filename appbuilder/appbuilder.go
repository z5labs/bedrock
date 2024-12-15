// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package appbuilder

import (
	"context"

	"github.com/z5labs/bedrock"
)

// Recover will wrap the give [bedrock.AppBuilder] with panic recovery.
// If the recovered panic value implements [error] then it will
// be directly returned. If it does not implement [error] then a
// [PanicError] will be returned instead.
func Recover[T any](builder bedrock.AppBuilder[T]) bedrock.AppBuilder[T] {
	return bedrock.AppBuilderFunc[T](func(ctx context.Context, cfg T) (_ bedrock.App, err error) {
		defer bedrock.Recover(&err)

		return builder.Build(ctx, cfg)
	})
}
