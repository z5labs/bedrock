// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package app

import (
	"context"
	"fmt"

	"github.com/z5labs/bedrock"
)

func ExampleRecover() {
	app := runFunc(func(ctx context.Context) error {
		panic("hello world")
		return nil
	})

	err := Recover(app).Run(context.Background())

	fmt.Println(err)
	// Output: recovered from panic: hello world
}

func ExampleWithLifecycleHooks() {
	var app bedrock.App = runFunc(func(ctx context.Context) error {
		return nil
	})

	postRun := LifecycleHookFunc(func(ctx context.Context) error {
		fmt.Println("ran post run hook")
		return nil
	})

	app = WithLifecycleHooks(app, Lifecycle{
		PostRun: postRun,
	})

	err := app.Run(context.Background())
	if err != nil {
		fmt.Println(err)
		return
	}

	// Output: ran post run hook
}

func ExampleWithLifecycleHooks_unrecoveredPanic() {
	var app bedrock.App = runFunc(func(ctx context.Context) error {
		panic("hello world")
		return nil
	})

	postRun := LifecycleHookFunc(func(ctx context.Context) error {
		fmt.Println("ran post run hook")
		return nil
	})

	app = WithLifecycleHooks(app, Lifecycle{
		PostRun: postRun,
	})

	run := func(ctx context.Context) error {
		// recover here so the panic doesn't crash the example
		defer func() {
			recover()
		}()

		return app.Run(ctx)
	}

	err := run(context.Background())
	if err != nil {
		fmt.Println(err)
		return
	}

	// Output: ran post run hook
}

func ExampleComposeLifecycleHooks() {
	var app bedrock.App = runFunc(func(ctx context.Context) error {
		return nil
	})

	app = WithLifecycleHooks(app, Lifecycle{
		PostRun: ComposeLifecycleHooks(
			LifecycleHookFunc(func(ctx context.Context) error {
				fmt.Println("one")
				return nil
			}),
			LifecycleHookFunc(func(ctx context.Context) error {
				fmt.Println("two")
				return nil
			}),
			LifecycleHookFunc(func(ctx context.Context) error {
				fmt.Println("three")
				return nil
			}),
		),
	})

	err := app.Run(context.Background())
	if err != nil {
		fmt.Println(err)
		return
	}

	// Output: one
	// two
	// three
}
