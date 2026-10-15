// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package bedrock_test

import (
	"context"
	"fmt"
	"os"

	"github.com/z5labs/bedrock"
)

// The main function of a program built on bedrock is one line. This example
// has no Output comment, so it is compiled but never run: running it would
// exit the test binary.
func Example_main() {
	app := bedrock.AppFunc(func(ctx context.Context, p bedrock.Process) bedrock.ExitStatus {
		ctx, stop := p.NotifyContext(ctx, os.Interrupt)
		defer stop()

		if _, err := fmt.Fprintln(p.Stdout, "hello, world"); err != nil {
			return bedrock.ExitWithCause(1, err)
		}

		<-ctx.Done()
		return bedrock.ExitWithCause(0, context.Cause(ctx))
	})

	// In package main:
	//
	//	func main() { bedrock.Main(app) }
	bedrock.Main(app)
}
