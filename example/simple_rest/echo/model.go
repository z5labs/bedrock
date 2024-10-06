// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package echo

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/swaggest/jsonschema-go"
	"github.com/swaggest/openapi-go/openapi3"
)

type Request struct {
	Msg string `json:"msg"`
}

func (Request) ContentType() string {
	return "application/json"
}

func (Request) Validate() error {
	return nil
}

func (req Request) OpenApiV3Schema() (*openapi3.Schema, error) {
	var reflector jsonschema.Reflector
	jsonSchema, err := reflector.Reflect(req)
	if err != nil {
		return nil, err
	}
	var schemaOrRef openapi3.SchemaOrRef
	schemaOrRef.FromJSONSchema(jsonSchema.ToSchemaOrBool())
	return schemaOrRef.Schema, nil
}

func (req Request) ReadFrom(r io.Reader) (int64, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return 0, err
	}
	err = json.Unmarshal(b, &req)
	return int64(len(b)), err
}

type Response struct {
	Msg string `json:"msg"`
}

func (Response) ContentType() string {
	return "application/json"
}

func (resp Response) OpenApiV3Schema() (*openapi3.Schema, error) {
	var reflector jsonschema.Reflector
	jsonSchema, err := reflector.Reflect(resp)
	if err != nil {
		return nil, err
	}
	var schemaOrRef openapi3.SchemaOrRef
	schemaOrRef.FromJSONSchema(jsonSchema.ToSchemaOrBool())
	return schemaOrRef.Schema, nil
}

func (resp *Response) WriteTo(w io.Writer) (int64, error) {
	b, err := json.Marshal(resp)
	if err != nil {
		return 0, err
	}
	return io.Copy(w, bytes.NewReader(b))
}
