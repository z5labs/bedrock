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
)

func ExampleRecover() {
	type MyConfig struct{}

	builder := bedrock.AppBuilderFunc[MyConfig](func(ctx context.Context, cfg MyConfig) (bedrock.App, error) {
		panic("hello world")
		return nil, nil
	})

	_, err := Recover(builder).Build(context.Background(), MyConfig{})

	var perr bedrock.PanicError
	if !errors.As(err, &perr) {
		fmt.Println("should be a panic error.")
		return
	}

	fmt.Println(perr.Value)
	// Output: hello world
}

func ExampleRecover_errorValue() {
	type MyConfig struct{}

	builder := bedrock.AppBuilderFunc[MyConfig](func(ctx context.Context, cfg MyConfig) (bedrock.App, error) {
		panic(errors.New("hello world"))
		return nil, nil
	})

	_, err := Recover(builder).Build(context.Background(), MyConfig{})

	var perr bedrock.PanicError
	if !errors.As(err, &perr) {
		fmt.Println("should be a panic error.")
		return
	}

	fmt.Println(perr.Unwrap())
	// Output: hello world
}
