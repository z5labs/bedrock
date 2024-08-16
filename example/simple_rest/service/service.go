// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"

	"github.com/z5labs/bedrock"
	"github.com/z5labs/bedrock/pkg/app"
	"github.com/z5labs/bedrock/rest"
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
	restApp := rest.NewApp(
		rest.ListenOn(cfg.Http.Port),
		rest.Handle(
			http.MethodPost,
			"/echo",
			echoHandler{},
		),
	)

	app := app.WithSignalNotifications(restApp, os.Interrupt, os.Kill)
	return app, nil
}

type echoHandler struct{}

type EchoRequest struct {
	Msg string `json:"msg"`
}

func (EchoRequest) ContentType() string {
	return "application/json"
}

func (req *EchoRequest) UnmarshalBinary(b []byte) error {
	return json.Unmarshal(b, req)
}

type EchoResponse struct {
	Msg string `json:"msg"`
}

func (EchoResponse) ContentType() string {
	return "application/json"
}

func (resp EchoResponse) MarshalBinary() ([]byte, error) {
	return json.Marshal(resp)
}

func (echoHandler) Handle(ctx context.Context, req EchoRequest) (EchoResponse, error) {
	return EchoResponse{Msg: req.Msg}, nil
}
