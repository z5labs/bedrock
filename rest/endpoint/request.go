// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package endpoint

import (
	"context"
	"encoding/json"
	"io"

	"github.com/swaggest/jsonschema-go"
	"github.com/swaggest/openapi-go/openapi3"
)

// Request
type Request[T any] interface {
	*T

	ContentTyper
	Validator
	OpenApiV3Schemaer
	io.ReaderFrom
}

type jsonRequestHandler[Req, Resp any] struct {
	inner Handler[Req, Resp]
}

// ConsumesJson
func ConsumesJson[Req, Resp any](h Handler[Req, Resp]) Handler[JsonRequest[Req], Resp] {
	return &jsonRequestHandler[Req, Resp]{
		inner: h,
	}
}

func (h *jsonRequestHandler[Req, Resp]) Handle(ctx context.Context, req *JsonRequest[Req]) (*Resp, error) {
	return h.inner.Handle(ctx, &req.inner)
}

// JsonRequest
type JsonRequest[T any] struct {
	inner T
}

// ContentType implements the [ContentTyper] interface.
func (JsonRequest[T]) ContentType() string {
	return "application/json"
}

// Validate implements the [Validator] interface.
func (req JsonRequest[T]) Validate() error {
	iv, ok := any(req.inner).(Validator)
	if !ok {
		return nil
	}
	return iv.Validate()
}

// OpenApiV3Schema implements the [OpenApiV3Schemaer] interface.
func (JsonRequest[T]) OpenApiV3Schema() (*openapi3.Schema, error) {
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

// ReadFrom implements the [io.ReaderFrom] interface.
func (req *JsonRequest[T]) ReadFrom(r io.Reader) (int64, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return 0, err
	}
	err = json.Unmarshal(b, &req.inner)
	return int64(len(b)), err
}
