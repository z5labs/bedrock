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

	"github.com/swaggest/jsonschema-go"
	"github.com/swaggest/openapi-go/openapi3"
)

// Response
type Response[T any] interface {
	*T

	ContentTyper
	OpenApiV3Schemaer
	io.WriterTo
}

// EmptyResponse
type EmptyResponse struct{}

// ContentType implements the [ContentTyper] interface.
func (EmptyResponse) ContentType() string {
	return ""
}

// OpenApiV3Schema implements the [OpenApiV3Schemaer] interface.
func (EmptyResponse) OpenApiV3Schema() (*openapi3.Schema, error) {
	return nil, nil
}

// WriteTo implements the [io.WriterTo] interface.
func (EmptyResponse) WriteTo(w io.Writer) (int64, error) {
	return 0, nil
}

// RequestOnlyHandler
type RequestOnlyHandler[Req any] interface {
	Handle(context.Context, *Req) error
}

// EmptyResponseHandler wraps a given [RequestOnlyHandler] into a complete [Handler]
// which does not return a response body.
type EmptyResponseHandler[Req any] struct {
	inner RequestOnlyHandler[Req]
}

// ProducesNothing constructs a [EmptyResponseHandler] from the given [RequestOnlyHandler].
func ProducesNothing[Req any](h RequestOnlyHandler[Req]) *EmptyResponseHandler[Req] {
	return &EmptyResponseHandler[Req]{
		inner: h,
	}
}

// Handle implements the [Handler] interface.
func (h *EmptyResponseHandler[Req]) Handle(ctx context.Context, req *Req) (*EmptyResponse, error) {
	err := h.inner.Handle(ctx, req)
	if err != nil {
		return nil, err
	}
	return &EmptyResponse{}, nil
}

// JsonResponseHandler wraps a given [Handler] and handles writing the underlying
// response type, Resp, to JSON.
type JsonResponseHandler[Req, Resp any] struct {
	inner Handler[Req, Resp]
}

// ProducesJson constructs a [JsonResponseHandler] from the given [Handler].
func ProducesJson[Req, Resp any](h Handler[Req, Resp]) *JsonResponseHandler[Req, Resp] {
	return &JsonResponseHandler[Req, Resp]{
		inner: h,
	}
}

// Handle implements the [Handler] interface.
func (h *JsonResponseHandler[Req, Resp]) Handle(ctx context.Context, req *Req) (*JsonResponse[Resp], error) {
	resp, err := h.inner.Handle(ctx, req)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, ErrNilHandlerResponse
	}
	return &JsonResponse[Resp]{inner: resp}, nil
}

// JsonResponse
type JsonResponse[T any] struct {
	inner *T
}

// ContentType implements the [ContentTyper] interface.
func (*JsonResponse[T]) ContentType() string {
	return "application/json"
}

// OpenApiV3Schema implements the [OpenApiV3Schemaer] interface.
func (JsonResponse[T]) OpenApiV3Schema() (*openapi3.Schema, error) {
	var reflector jsonschema.Reflector
	var t T
	jsonSchema, err := reflector.Reflect(t)
	if err != nil {
		return nil, err
	}
	var schemaOrRef openapi3.SchemaOrRef
	schemaOrRef.FromJSONSchema(jsonSchema.ToSchemaOrBool())
	return schemaOrRef.Schema, nil
}

// WriteTo implements the [io.WriterTo] interface.
func (resp *JsonResponse[T]) WriteTo(w io.Writer) (int64, error) {
	b, err := json.Marshal(resp.inner)
	if err != nil {
		return 0, err
	}
	return io.Copy(w, bytes.NewReader(b))
}
