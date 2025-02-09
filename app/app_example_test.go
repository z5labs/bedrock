// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package app

import (
	"context"
	"fmt"
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
