// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package appbuilder

import (
	"context"
	"errors"
	"fmt"

	"github.com/z5labs/bedrock"
	"github.com/z5labs/bedrock/lifecycle"
)

func ExampleRecover() {
	type MyConfig struct{}

	builder := bedrock.AppBuilderFunc[MyConfig](func(ctx context.Context, cfg MyConfig) (bedrock.App, error) {
		panic("hello world")
		return nil, nil
	})

	_, err := Recover(builder).Build(context.Background(), MyConfig{})
	fmt.Println(err)
	// Output: recovered from panic: hello world
}

func ExampleLifecycleContext() {
	type MyConfig struct{}

	builder := bedrock.AppBuilderFunc[MyConfig](func(ctx context.Context, cfg MyConfig) (bedrock.App, error) {
		lc, ok := lifecycle.FromContext(ctx)
		if !ok {
			return nil, errors.New("expected lifecycle in build context")
		}

		lc.OnPostRun(lifecycle.HookFunc(func(ctx context.Context) error {
			fmt.Println("ran post run hook")
			return nil
		}))

		app := appFunc(func(ctx context.Context) error {
			return nil
		})
		return app, nil
	})

	app, err := LifecycleContext(builder, &lifecycle.Context{}).Build(context.Background(), MyConfig{})
	if err != nil {
		fmt.Println(err)
		return
	}

	err = app.Run(context.Background())
	if err != nil {
		fmt.Println(err)
		return
	}

	// Output: ran post run hook
}
