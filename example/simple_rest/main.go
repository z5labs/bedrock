// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package main

import (
	"context"
	"embed"
	"log/slog"

	"github.com/z5labs/bedrock"
	"github.com/z5labs/bedrock/example/simple_rest/service"
	"github.com/z5labs/bedrock/pkg/config"
)

//go:embed config.yaml
var configDir embed.FS

func main() {
	err := bedrock.Run(
		context.Background(),
		bedrock.AppBuilderFunc[service.Config](service.Init),
		config.FromYaml(
			config.NewFileReader(configDir, "config.yaml"),
		),
	)
	if err != nil {
		slog.Default().Error("failed to run", slog.String("error", err.Error()))
	}
}
