// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package main

import (
	"context"
	"log"
	"os"

	"github.com/z5labs/bedrock"
	"github.com/z5labs/bedrock/example/http/rest/ecommerce/app"
	"github.com/z5labs/bedrock/example/http/rest/ecommerce/services/cart"
	httprt "github.com/z5labs/bedrock/runtime/http"
)

func main() {
	cartSvc := cart.NewService()

	rt := app.New(cartSvc)

	runner := bedrock.NotifyOnSignal(
		bedrock.RecoverPanics(
			bedrock.DefaultRunner[httprt.Runtime](),
		),
		os.Interrupt,
	)

	if err := runner.Run(context.Background(), rt); err != nil {
		log.Fatal(err)
	}
}
