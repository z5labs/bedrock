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
	"net/http"
	"os"

	"github.com/z5labs/bedrock"
	brhttp "github.com/z5labs/bedrock/http"
	"github.com/z5labs/bedrock/pkg/config"
)

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

	rt := brhttp.NewRuntime(
		brhttp.ListenOnPort(8080),
		brhttp.LogHandler(logHandler),
		brhttp.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "Hello, world")
		}),
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
