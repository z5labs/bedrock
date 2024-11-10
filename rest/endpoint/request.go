// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package endpoint

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/swaggest/jsonschema-go"
	"github.com/swaggest/openapi-go/openapi3"
	"gopkg.in/yaml.v3"
)

// RequestReader
type RequestReader interface {
	ReadRequest(r *http.Request) error
}

// Request
type Request[T any] interface {
	*T

	ContentTyper
	Validator
	OpenApiV3Schemaer
	RequestReader
}

// EmptyRequest
type EmptyRequest struct{}

// ContentType implements [ContentTyper] interface.
func (EmptyRequest) ContentType() string {
	return ""
}

// Validate implements the [Validator] interface.
func (EmptyRequest) Validate() error {
	return nil
}

// OpenApiV3Schema implements the [OpenApiV3Schemaer] interface.
func (EmptyRequest) OpenApiV3Schema() (*openapi3.Schema, error) {
	return nil, nil
}

// ReadRequest implements the [RequestReader] interface.
func (*EmptyRequest) ReadRequest(r *http.Request) error {
	return nil
}

// ResponseOnlyHandler
type ResponseOnlyHandler[Resp any] interface {
	Handle(context.Context) (*Resp, error)
}

// EmptyRequestHandler wraps a given [ResponseOnlyHandler] into a complete [Handler]
// which expects an empty request body.
type EmptyRequestHandler[Resp any] struct {
	inner ResponseOnlyHandler[Resp]
}

// ConsumesNothing constructs a [EmptyRequestHandler] from the given [ResponseOnlyHandler].
func ConsumesNothing[Resp any](h ResponseOnlyHandler[Resp]) *EmptyRequestHandler[Resp] {
	return &EmptyRequestHandler[Resp]{
		inner: h,
	}
}

// Handle implements the [Handler] interface.
func (h *EmptyRequestHandler[Resp]) Handle(ctx context.Context, _ *EmptyRequest) (*Resp, error) {
	return h.inner.Handle(ctx)
}

// JsonRequestHandler wraps a given [Handler] and handles reading the underlying
// request type, Req, from JSON.
type JsonRequestHandler[Req, Resp any] struct {
	inner Handler[Req, Resp]
}

// ConsumesJson constructs a [JsonRequestHandler] from the given [Handler].
func ConsumesJson[Req, Resp any](h Handler[Req, Resp]) *JsonRequestHandler[Req, Resp] {
	return &JsonRequestHandler[Req, Resp]{
		inner: h,
	}
}

// Handle implements the [Handler] interface.
func (h *JsonRequestHandler[Req, Resp]) Handle(ctx context.Context, req *JsonRequest[Req]) (*Resp, error) {
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

// ReadRequest implements the [RequestReader] interface.
func (req *JsonRequest[T]) ReadRequest(r *http.Request) (err error) {
	defer close(&err, r.Body)

	var b []byte
	b, err = io.ReadAll(r.Body)
	if err != nil {
		return
	}
	err = json.Unmarshal(b, &req.inner)
	return
}

// YamlRequestHandler wraps a given [Handler] and handles reading the underlying
// request type, Req, from YAML.
type YamlRequestHandler[Req, Resp any] struct {
	inner Handler[Req, Resp]
}

// ConsumesYaml constructs a [YamlRequestHandler] from the given [Handler].
func ConsumesYaml[Req, Resp any](h Handler[Req, Resp]) *YamlRequestHandler[Req, Resp] {
	return &YamlRequestHandler[Req, Resp]{
		inner: h,
	}
}

// Handle implements the [Handler] interface.
func (h *YamlRequestHandler[Req, Resp]) Handle(ctx context.Context, req *YamlRequest[Req]) (*Resp, error) {
	return h.inner.Handle(ctx, &req.inner)
}

// YamlRequest
type YamlRequest[T any] struct {
	inner T
}

// ContentType implements the [ContentTyper] interface.
func (YamlRequest[T]) ContentType() string {
	return "application/yaml"
}

// Validate implements the [Validator] interface.
func (req YamlRequest[T]) Validate() error {
	iv, ok := any(req.inner).(Validator)
	if !ok {
		return nil
	}
	return iv.Validate()
}

// OpenApiV3Schema implements the [OpenApiV3Schemaer] interface.
func (YamlRequest[T]) OpenApiV3Schema() (*openapi3.Schema, error) {
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

// ReadRequest implements the [RequestReader] interface.
func (req *YamlRequest[T]) ReadRequest(r *http.Request) (err error) {
	defer close(&err, r.Body)

	var b []byte
	b, err = io.ReadAll(r.Body)
	if err != nil {
		return
	}
	err = yaml.Unmarshal(b, &req.inner)
	return
}

func close(err *error, c io.Closer) {
	closeErr := c.Close()
	if closeErr == nil {
		return
	}
	if *err == nil {
		*err = closeErr
		return
	}
	*err = errors.Join(*err, closeErr)
}
