// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package appbuilder

import (
	"context"

	"github.com/z5labs/bedrock"
	"github.com/z5labs/bedrock/config"
	"github.com/z5labs/bedrock/internal/try"
)

// Recover will wrap the given [bedrock.AppBuilder] with panic recovery.
func Recover[T any](builder bedrock.AppBuilder[T]) bedrock.AppBuilder[T] {
	return bedrock.AppBuilderFunc[T](func(ctx context.Context, cfg T) (_ bedrock.App, err error) {
		defer try.Recover(&err)

		return builder.Build(ctx, cfg)
	})
}

// FromConfig returns a [bedrock.AppBuilder] which unmarshals
// the given [bedrock.AppBuilder]s input type, T, from a [config.Source].
func FromConfig[T any](builder bedrock.AppBuilder[T]) bedrock.AppBuilder[config.Source] {
	return bedrock.AppBuilderFunc[config.Source](func(ctx context.Context, src config.Source) (bedrock.App, error) {
		m, err := config.Read(src)
		if err != nil {
			return nil, err
		}

		var cfg T
		err = m.Unmarshal(&cfg)
		if err != nil {
			return nil, err
		}

		return builder.Build(ctx, cfg)
	})
}
