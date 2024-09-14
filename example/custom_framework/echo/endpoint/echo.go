// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package endpoint

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/z5labs/bedrock/example/custom_framework/framework/rest"
)

type echoHandler struct {
	log *slog.Logger
}

func Echo(log *slog.Logger) rest.Endpoint {
	h := &echoHandler{
		log: log,
	}

	return rest.Endpoint{
		Method:    http.MethodPost,
		Path:      "/echo",
		Operation: rest.NewOperation(h),
	}
}

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

func (resp *EchoResponse) MarshalBinary() ([]byte, error) {
	return json.Marshal(resp)
}

func (h *echoHandler) Handle(ctx context.Context, req *EchoRequest) (*EchoResponse, error) {
	h.log.InfoContext(ctx, "echoing back received message to client", slog.String("echo_msg", req.Msg))
	resp := &EchoResponse{Msg: req.Msg}
	return resp, nil
}
