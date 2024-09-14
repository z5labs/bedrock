// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package app

import (
	"context"

	"github.com/z5labs/bedrock/example/custom_framework/echo/endpoint"

	"github.com/z5labs/bedrock/example/custom_framework/framework"
	"github.com/z5labs/bedrock/example/custom_framework/framework/rest"
)

type Config struct {
	rest.Config `config:",squash"`
}

func Init(ctx context.Context, cfg Config) (framework.App, error) {
	log := framework.Logger(cfg.Logging)

	app := rest.NewApp(
		rest.OpenApi(cfg.OpenApi),
		rest.OTel(cfg.OTel),
		rest.HttpServer(cfg.Http),
		rest.WithEndpoint(endpoint.Echo(log)),
	)
	return app, nil
}
