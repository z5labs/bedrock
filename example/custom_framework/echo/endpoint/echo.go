// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package endpoint

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/z5labs/bedrock/example/custom_framework/framework/rest"

	"github.com/z5labs/bedrock/rest/endpoint"
)

type echoHandler struct {
	log *slog.Logger
}

func Echo(log *slog.Logger) rest.Endpoint {
	h := &echoHandler{
		log: log,
	}

	return rest.Endpoint{
		Method: http.MethodPost,
		Path:   "/echo",
		Operation: rest.NewOperation(
			endpoint.ConsumesJson(
				endpoint.ProducesJson(h),
			),
		),
	}
}

type EchoRequest struct {
	Msg string `json:"msg"`
}

type EchoResponse struct {
	Msg string `json:"msg"`
}

func (h *echoHandler) Handle(ctx context.Context, req *EchoRequest) (*EchoResponse, error) {
	h.log.InfoContext(ctx, "echoing back received message to client", slog.String("echo_msg", req.Msg))
	resp := &EchoResponse{Msg: req.Msg}
	return resp, nil
}
