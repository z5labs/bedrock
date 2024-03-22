// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package bedrock

import (
	"context"
	"fmt"
	"strings"

	"github.com/z5labs/bedrock/pkg/config/configtmpl"
)

func ExampleApp_Run() {
	r := strings.NewReader(`hello: {{env "HELLO" | default "world"}}`)

	app := New(
		Name("example"),
		ConfigTemplateFunc("env", configtmpl.Env),
		ConfigTemplateFunc("default", configtmpl.Default),
		Config(r),
		WithRuntimeBuilderFunc(func(ctx context.Context) (Runtime, error) {
			m := ConfigFromContext(ctx)
			var cfg struct {
				Hello string `config:"hello"`
			}
			err := m.Unmarshal(&cfg)
			if err != nil {
				return nil, err
			}

			rt := runtimeFunc(func(ctx context.Context) error {
				fmt.Printf("hello, %s\n", cfg.Hello)
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
