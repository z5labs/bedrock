// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package main

import (
	"context"
	"embed"
	"fmt"
	"log/slog"
	"os"

	"github.com/z5labs/bedrock"
	"github.com/z5labs/bedrock/pkg/config"
	"github.com/z5labs/bedrock/queue"
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

type Config struct {
	Logging struct {
		Level slog.Level `config:"level"`
	} `config:"logging"`
}

func initRuntime(ctx context.Context, cfg Config) (bedrock.App, error) {
	logHandler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		AddSource: true,
		Level:     cfg.Logging.Level,
	})

	consumer := &intGenerator{n: 0}

	processor := evenOrOdd{}

	rt := queue.Sequential[int](
		consumer,
		processor,
		queue.LogHandler(logHandler),
	)
	return rt, nil
}

//go:embed config.yaml
var configDir embed.FS

func main() {
	err := bedrock.Run(
		context.Background(),
		bedrock.AppBuilderFunc[Config](initRuntime),
		config.FromYaml(
			config.NewFileReader(configDir, "config.yaml"),
		),
	)
	if err != nil {
		slog.Default().Error("failed to run", slog.String("error", err.Error()))
	}
}
