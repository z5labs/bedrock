// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package bedrock

import (
	"context"
	"fmt"

	"github.com/z5labs/bedrock/pkg/otelconfig"
)

func ExampleApp_Run() {
	app := New(
		Name("example"),
		WithRuntimeBuilderFunc(func(ctx context.Context) (Runtime, error) {
			life := LifecycleFromContext(ctx)
			WithTracerProvider(life, otelconfig.Local())
			rt := runtimeFunc(func(ctx context.Context) error {
				fmt.Println("hello, world")
				return nil
			})
			return rt, nil
		}),
	)

	err := app.Run()
	if err != nil {
		fmt.Println(err)
		return
	}
	//Output: hello, world
}
