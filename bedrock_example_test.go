// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package bedrock

import (
	"context"
	"fmt"
	"strings"

	"github.com/z5labs/bedrock/pkg/config"
)

type appFunc func(context.Context) error

func (f appFunc) Run(ctx context.Context) error {
	return f(ctx)
}

func ExampleRun() {
	r := strings.NewReader(`hello: world`)

	// Define a custom config struct which aligns with your
	// the format of your config.Source(s).
	type MyConfig struct {
		Hello string `config:"hello"`
	}

	// Define a AppBuilder.
	builder := AppBuilderFunc[MyConfig](func(ctx context.Context, cfg MyConfig) (App, error) {
		// Inside your AppBuilder.Build is where you should be initializing
		// all dependencies of your code e.g. backend API clients, DB connections, etc.

		// Lastly, initialize something which implements the bedrock.App interface.
		app := appFunc(func(c context.Context) error {
			fmt.Println("hello,", cfg.Hello)
			return nil
		})

		return app, nil
	})

	err := Run(context.Background(), builder, config.FromYaml(r))
	if err != nil {
		fmt.Println(err)
		return
	}
	//Output: hello, world
}
