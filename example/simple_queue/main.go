// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package main

import (
	"context"
	"fmt"

	"github.com/z5labs/app"
	"github.com/z5labs/app/queue"
)

type intGenerator struct {
	n int
}

func (g *intGenerator) Consume(ctx context.Context) (*queue.Item[int], error) {
	item := &queue.Item[int]{
		Value: g.n,
	}
	g.n += 1
	return item, nil
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

func main() {
	app.New(
		app.RuntimeBuilderFunc(func(bc app.BuildContext) (app.Runtime, error) {
			rt := queue.NewRuntime(
				queue.Pipe[int](
					&intGenerator{n: 0},
					evenOrOdd{},
				),
			)
			return rt, nil
		}),
	).Run()
}
