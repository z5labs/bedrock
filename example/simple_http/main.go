// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/z5labs/app"
	apphttp "github.com/z5labs/app/http"
	"github.com/z5labs/app/pkg/otelconfig"
)

func initRuntime(bc app.BuildContext) (app.Runtime, error) {
	logHandler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{AddSource: true})

	rt := apphttp.NewRuntime(
		apphttp.ListenOnPort(8080),
		apphttp.LogHandler(logHandler),
		apphttp.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "Hello, world")
		}),
	)
	return rt, nil
}

func main() {
	app.New(
		app.InitTracerProvider(otelconfig.Local(
			otelconfig.ServiceName("simple_http"),
		)),
		app.WithRuntimeBuilderFunc(initRuntime),
	).Run()
}
