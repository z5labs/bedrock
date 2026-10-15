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

func Example() {
	app := bedrock.AppFunc(func(ctx bedrock.Context) bedrock.ExitStatus {
		sigCtx, stop := ctx.NotifyOnInterrupt(context.Background(), os.Interrupt)
		defer stop()

		select {
		case <-sigCtx.Done():
			return bedrock.ExitWithCause(1, context.Cause(sigCtx))
		default:
		}

		if _, err := fmt.Fprintln(ctx.Stdout, "hello, world"); err != nil {
			return bedrock.ExitWithCause(1, err)
		}
		return bedrock.Exit(0)
	})

	status := app.Run(bedrock.OsContext())
	fmt.Println(status.Code())
	// Output:
	// hello, world
	// 0
}
