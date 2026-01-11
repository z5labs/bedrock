// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package bedrock

import (
	"context"
	"fmt"
	"os"
	"syscall"
)

func Example() {
	buildRuntime := BuilderFunc[RuntimeFunc](func(ctx context.Context) (RuntimeFunc, error) {
		rt := func(ctx context.Context) error {
			fmt.Println("hello from runtime")
			return nil
		}

		return rt, nil
	})

	err := NotifyOnSignal(
		RecoverPanics(
			DefaultRunner[RuntimeFunc](),
		),
		os.Interrupt,
		os.Kill,
		syscall.SIGTERM,
	).Run(
		context.Background(),
		buildRuntime,
	)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Output:
	// hello from runtime
}
