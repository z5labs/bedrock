// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/z5labs/bedrock"
	brhttp "github.com/z5labs/bedrock/http"
	"github.com/z5labs/bedrock/pkg/otelconfig"
)

func initRuntime(ctx context.Context) (bedrock.Runtime, error) {
	logHandler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{AddSource: true})

	rt := brhttp.NewRuntime(
		brhttp.ListenOnPort(8080),
		brhttp.LogHandler(logHandler),
		brhttp.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "Hello, world")
		}),
	)
	return rt, nil
}

func main() {
	bedrock.New(
		bedrock.InitTracerProvider(func(_ context.Context) (otelconfig.Initializer, error) {
			return otelconfig.Local(
				otelconfig.ServiceName("simple_http"),
			), nil
		}),
		bedrock.WithRuntimeBuilderFunc(initRuntime),
	).Run()
}
