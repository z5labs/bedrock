// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package app

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"

	"github.com/z5labs/bedrock"
	"github.com/z5labs/bedrock/example/simple_rest/echo"
	"github.com/z5labs/bedrock/pkg/app"
	"github.com/z5labs/bedrock/rest"
	"github.com/z5labs/bedrock/rest/endpoint"
)

type Config struct {
	Logging struct {
		Level slog.Level `config:"level"`
	} `config:"logging"`

	Http struct {
		Port uint `config:"port"`
	} `config:"http"`
}

func Init(ctx context.Context, cfg Config) (bedrock.App, error) {
	logHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level:     cfg.Logging.Level,
		AddSource: true,
	})

	echoService := echo.NewService(
		echo.LogHandler(logHandler),
	)

	ls, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.Http.Port))
	if err != nil {
		return nil, err
	}

	restApp := rest.NewApp(
		rest.Listener(ls),
		rest.Endpoint(
			http.MethodPost,
			"/echo",
			endpoint.NewOperation(
				echoService,
				endpoint.Headers(
					endpoint.Header{
						Name: "Authorization",
					},
				),
			),
		),
	)

	app := app.WithSignalNotifications(restApp, os.Interrupt, os.Kill)
	return app, nil
}
