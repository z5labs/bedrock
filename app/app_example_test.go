// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package app

import (
	"context"
	"fmt"

	"github.com/z5labs/bedrock/lifecycle"
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

func ExamplePostRun() {
	app := runFunc(func(ctx context.Context) error {
		return nil
	})

	hook := lifecycle.HookFunc(func(ctx context.Context) error {
		fmt.Println("hook ran")
		return nil
	})

	err := PostRun(app, hook).Run(context.Background())
	if err != nil {
		fmt.Println(err)
		return
	}

	// Output: hook ran
}
