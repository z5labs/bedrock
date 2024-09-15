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

	var perr PanicError
	if !errors.As(err, &perr) {
		fmt.Println("should be a panic error since \"hello world\" does not implement error interface.")
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
	if errors.Is(err, PanicError{}) {
		fmt.Println("should not be a panic error since errors.New() does implement error interface.")
		return
	}

	fmt.Println(err)
	// Output: hello world
}
