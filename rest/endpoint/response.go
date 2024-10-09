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
