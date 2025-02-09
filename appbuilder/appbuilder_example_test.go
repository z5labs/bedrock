// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package appbuilder

import (
	"context"
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
	fmt.Println(err)
	// Output: recovered from panic: hello world
}
