// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/z5labs/app"
	"github.com/z5labs/app/pkg/otelconfig"
	"github.com/z5labs/app/queue"
)

type intGenerator struct {
	n int
}

func (g *intGenerator) Consume(ctx context.Context) (int, error) {
	g.n += 1
	return g.n, nil
}

type evenOrOdd struct{}

func (p evenOrOdd) Process(ctx context.Context, n int) error {
	if n%2 == 0 {
		fmt.Println("even")
		return nil
	}
	fmt.Println("odd")
	return nil
}

func initRuntime(bc app.BuildContext) (app.Runtime, error) {
	logHandler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{AddSource: true})

	consumer := &intGenerator{n: 0}

	processor := evenOrOdd{}

	rt := queue.NewRuntime(
		queue.LogHandler(logHandler),
		queue.InitTracerProvider(otelconfig.Stdout),
		queue.Pipe[int](consumer, processor),
	)
	return rt, nil
}

func main() {
	app.New(
		app.WithRuntimeBuilderFunc(initRuntime),
	).Run()
}
