// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package appbuilder

import (
	"context"
	"fmt"
	"strings"

	"github.com/z5labs/bedrock"
	"github.com/z5labs/bedrock/config"
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

func ExampleFromConfig() {
	type MyConfig struct {
		Hello string `config:"hello"`
	}

	builder := bedrock.AppBuilderFunc[MyConfig](func(ctx context.Context, cfg MyConfig) (bedrock.App, error) {
		fmt.Println(cfg.Hello)
		return nil, nil
	})

	cfgSrc := config.FromYaml(strings.NewReader(`hello: world`))
	_, err := FromConfig(builder).Build(context.Background(), cfgSrc)
	if err != nil {
		fmt.Println(err)
		return
	}
	// Output: world
}
