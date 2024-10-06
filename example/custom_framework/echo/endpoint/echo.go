// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package endpoint

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/z5labs/bedrock/example/custom_framework/framework/rest"

	"github.com/swaggest/jsonschema-go"
	"github.com/swaggest/openapi-go/openapi3"
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

func (EchoRequest) Validate() error {
	return nil
}

func (req EchoRequest) OpenApiV3Schema() (*openapi3.Schema, error) {
	var reflector jsonschema.Reflector
	jsonSchema, err := reflector.Reflect(req)
	if err != nil {
		return nil, err
	}
	var schemaOrRef openapi3.SchemaOrRef
	schemaOrRef.FromJSONSchema(jsonSchema.ToSchemaOrBool())
	return schemaOrRef.Schema, nil
}

func (req *EchoRequest) ReadFrom(r io.Reader) (int64, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return 0, err
	}
	err = json.Unmarshal(b, &req)
	return int64(len(b)), err
}

type EchoResponse struct {
	Msg string `json:"msg"`
}

func (EchoResponse) ContentType() string {
	return "application/json"
}

func (resp EchoResponse) OpenApiV3Schema() (*openapi3.Schema, error) {
	var reflector jsonschema.Reflector
	jsonSchema, err := reflector.Reflect(resp)
	if err != nil {
		return nil, err
	}
	var schemaOrRef openapi3.SchemaOrRef
	schemaOrRef.FromJSONSchema(jsonSchema.ToSchemaOrBool())
	return schemaOrRef.Schema, nil
}

func (resp *EchoResponse) WriteTo(w io.Writer) (int64, error) {
	b, err := json.Marshal(resp)
	if err != nil {
		return 0, err
	}
	return io.Copy(w, bytes.NewReader(b))
}

func (h *echoHandler) Handle(ctx context.Context, req *EchoRequest) (*EchoResponse, error) {
	h.log.InfoContext(ctx, "echoing back received message to client", slog.String("echo_msg", req.Msg))
	resp := &EchoResponse{Msg: req.Msg}
	return resp, nil
}
