// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package rest

import (
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/swaggest/openapi-go/openapi3"
)

// ReadJSON composes JSON request body decoding onto the endpoint.
// The type parameter T must match the body type in the handler's Request[T].
func ReadJSON[T any](ep Endpoint) Endpoint {
	ep.bodyDecoder = func(r *http.Request) (any, error) {
		var body T
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return nil, &ValidationError{
				Param:   "body",
				Message: fmt.Sprintf("invalid JSON request body: %v", err),
			}
		}
		return body, nil
	}
	method := ep.method
	if method == "" {
		method = http.MethodPost
	}
	ep.specReqBody = func(refl *openapi3.Reflector, op *openapi3.Operation) error {
		return refl.SetRequest(op, new(T), method) //nolint:staticcheck
	}
	return ep
}

// ReadBinary composes binary request body decoding onto the endpoint.
// The handler's Request body type should be io.Reader.
func ReadBinary(contentType string, ep Endpoint) Endpoint {
	ep.bodyDecoder = func(r *http.Request) (any, error) {
		return r.Body, nil
	}
	ep.specReqBody = func(refl *openapi3.Reflector, op *openapi3.Operation) error {
		reqBody := openapi3.RequestBody{}
		reqBody.WithContentItem(contentType, openapi3.MediaType{
			Schema: &openapi3.SchemaOrRef{
				Schema: &openapi3.Schema{
					Type:   ptrSchemaType(openapi3.SchemaTypeString),
					Format: ptrString("binary"),
				},
			},
		})
		t := true
		reqBody.Required = &t
		op.WithRequestBody(openapi3.RequestBodyOrRef{RequestBody: &reqBody})
		return nil
	}
	return ep
}

// ReadFormFile composes multipart file field decoding onto the endpoint.
// The decoded io.ReadCloser is stored in the endpoint's body for access via req.Body().
// The caller is responsible for closing the returned io.ReadCloser.
func ReadFormFile(fieldName string, ep Endpoint) Endpoint {
	ep.bodyDecoder = func(r *http.Request) (any, error) {
		ct := r.Header.Get("Content-Type")
		mediaType, _, err := mime.ParseMediaType(ct)
		if err != nil || !strings.HasPrefix(mediaType, "multipart/") {
			return nil, &ValidationError{
				Param:   "body",
				Message: "expected multipart/form-data content type",
			}
		}
		file, _, err := r.FormFile(fieldName)
		if err != nil {
			return nil, &ValidationError{
				Param:   fieldName,
				Message: fmt.Sprintf("missing form file %q: %v", fieldName, err),
			}
		}
		return io.ReadCloser(file), nil
	}
	ep.specReqBody = func(refl *openapi3.Reflector, op *openapi3.Operation) error {
		reqBody := openapi3.RequestBody{}
		reqBody.WithContentItem("multipart/form-data", openapi3.MediaType{
			Schema: &openapi3.SchemaOrRef{
				Schema: &openapi3.Schema{
					Type: ptrSchemaType(openapi3.SchemaTypeObject),
					Properties: map[string]openapi3.SchemaOrRef{
						fieldName: {
							Schema: &openapi3.Schema{
								Type:   ptrSchemaType(openapi3.SchemaTypeString),
								Format: ptrString("binary"),
							},
						},
					},
				},
			},
		})
		t := true
		reqBody.Required = &t
		op.WithRequestBody(openapi3.RequestBodyOrRef{RequestBody: &reqBody})
		return nil
	}
	return ep
}

// ReadFormField composes a multipart form field decoding onto the endpoint.
// The decoded value of type T is stored as the endpoint's body.
func ReadFormField[T any](fieldName string, ep Endpoint) Endpoint {
	ep.bodyDecoder = func(r *http.Request) (any, error) {
		ct := r.Header.Get("Content-Type")
		mediaType, params, err := mime.ParseMediaType(ct)
		if err != nil || !strings.HasPrefix(mediaType, "multipart/") {
			return nil, &ValidationError{
				Param:   "body",
				Message: "expected multipart/form-data content type",
			}
		}
		reader := multipart.NewReader(r.Body, params["boundary"])
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, &ValidationError{
					Param:   fieldName,
					Message: fmt.Sprintf("error reading multipart form: %v", err),
				}
			}
			if part.FormName() == fieldName {
				data, err := io.ReadAll(part)
				part.Close() //nolint:errcheck
				if err != nil {
					return nil, err
				}
				val, err := parseValue[T](string(data))
				if err != nil {
					return nil, &ValidationError{
						Param:   fieldName,
						Message: fmt.Sprintf("invalid value for form field %q: %v", fieldName, err),
					}
				}
				return val, nil
			}
			part.Close() //nolint:errcheck
		}
		return nil, &ValidationError{
			Param:   fieldName,
			Message: fmt.Sprintf("form field %q not found", fieldName),
		}
	}
	ep.specReqBody = func(refl *openapi3.Reflector, op *openapi3.Operation) error {
		schema := schemaForType[T]()
		reqBody := openapi3.RequestBody{}
		reqBody.WithContentItem("multipart/form-data", openapi3.MediaType{
			Schema: &openapi3.SchemaOrRef{
				Schema: &openapi3.Schema{
					Type: ptrSchemaType(openapi3.SchemaTypeObject),
					Properties: map[string]openapi3.SchemaOrRef{
						fieldName: {Schema: schema},
					},
				},
			},
		})
		op.WithRequestBody(openapi3.RequestBodyOrRef{RequestBody: &reqBody})
		return nil
	}
	return ep
}

func ptrString(s string) *string {
	return &s
}
