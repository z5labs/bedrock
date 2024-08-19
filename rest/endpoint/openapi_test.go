// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package endpoint

import (
	"context"
	"encoding/json"
	"net/http"
	"path"
	"strconv"
	"strings"
	"testing"

	"github.com/z5labs/bedrock/pkg/ptr"

	"github.com/stretchr/testify/assert"
	"github.com/swaggest/openapi-go/openapi3"
)

func TestEndpoint_OpenApi(t *testing.T) {
	t.Run("will set non-required header parameter", func(t *testing.T) {
		t.Run("if a header is provided with the Headers option", func(t *testing.T) {
			method := strings.ToLower(http.MethodPost)
			pattern := "/"
			header := Header{
				Name:     "MyHeader",
				Required: true,
			}

			e := New(
				method,
				pattern,
				HandlerFunc[Empty, Empty](func(_ context.Context, _ Empty) (Empty, error) {
					return Empty{}, nil
				}),
				Headers(header),
			)

			refSpec := &openapi3.Spec{
				Openapi: "3.0.3",
			}
			e.OpenApi(refSpec)

			b, err := json.Marshal(refSpec)
			if !assert.Nil(t, err) {
				return
			}

			var spec openapi3.Spec
			err = json.Unmarshal(b, &spec)
			if !assert.Nil(t, err) {
				return
			}

			pathItems := spec.Paths.MapOfPathItemValues
			if !assert.Len(t, pathItems, 1) {
				return
			}
			if !assert.Contains(t, pathItems, pattern) {
				return
			}

			ops := pathItems[pattern].MapOfOperationValues
			if !assert.Len(t, ops, 1) {
				return
			}
			if !assert.Contains(t, ops, method) {
				return
			}

			op := ops[method]
			params := op.Parameters
			if !assert.Len(t, params, 1) {
				return
			}

			param := params[0].Parameter
			if !assert.NotNil(t, param) {
				return
			}
			if !assert.Equal(t, openapi3.ParameterInHeader, param.In) {
				return
			}
			if !assert.Equal(t, header.Name, param.Name) {
				return
			}
			if !assert.Equal(t, header.Required, ptr.Deref(param.Required)) {
				return
			}
		})
	})

	t.Run("will set required header parameter", func(t *testing.T) {
		t.Run("if a header is provided with the Headers option", func(t *testing.T) {
			method := strings.ToLower(http.MethodPost)
			pattern := "/"
			header := Header{
				Name:     "MyHeader",
				Required: true,
			}

			e := New(
				method,
				pattern,
				HandlerFunc[Empty, Empty](func(_ context.Context, _ Empty) (Empty, error) {
					return Empty{}, nil
				}),
				Headers(header),
			)

			refSpec := &openapi3.Spec{
				Openapi: "3.0.3",
			}
			e.OpenApi(refSpec)

			b, err := json.Marshal(refSpec)
			if !assert.Nil(t, err) {
				return
			}

			var spec openapi3.Spec
			err = json.Unmarshal(b, &spec)
			if !assert.Nil(t, err) {
				return
			}

			pathItems := spec.Paths.MapOfPathItemValues
			if !assert.Len(t, pathItems, 1) {
				return
			}
			if !assert.Contains(t, pathItems, pattern) {
				return
			}

			ops := pathItems[pattern].MapOfOperationValues
			if !assert.Len(t, ops, 1) {
				return
			}
			if !assert.Contains(t, ops, method) {
				return
			}

			op := ops[method]
			params := op.Parameters
			if !assert.Len(t, params, 1) {
				return
			}

			param := params[0].Parameter
			if !assert.NotNil(t, param) {
				return
			}
			if !assert.Equal(t, openapi3.ParameterInHeader, param.In) {
				return
			}
			if !assert.Equal(t, header.Name, param.Name) {
				return
			}
			if !assert.Equal(t, header.Required, ptr.Deref(param.Required)) {
				return
			}
		})
	})

	t.Run("will set request body type", func(t *testing.T) {
		t.Run("if the request type implements ContentTyper interface", func(t *testing.T) {
			method := strings.ToLower(http.MethodPost)
			pattern := "/"

			e := New(
				method,
				pattern,
				HandlerFunc[JsonContent, Empty](func(_ context.Context, _ JsonContent) (Empty, error) {
					return Empty{}, nil
				}),
			)

			refSpec := &openapi3.Spec{
				Openapi: "3.0.3",
			}
			e.OpenApi(refSpec)

			b, err := json.Marshal(refSpec)
			if !assert.Nil(t, err) {
				return
			}

			var spec openapi3.Spec
			err = json.Unmarshal(b, &spec)
			if !assert.Nil(t, err) {
				return
			}

			pathItems := spec.Paths.MapOfPathItemValues
			if !assert.Len(t, pathItems, 1) {
				return
			}
			if !assert.Contains(t, pathItems, pattern) {
				return
			}

			ops := pathItems[pattern].MapOfOperationValues
			if !assert.Len(t, ops, 1) {
				return
			}
			if !assert.Contains(t, ops, method) {
				return
			}

			reqBodyOrRef := ops[method].RequestBody
			if !assert.NotNil(t, reqBodyOrRef) {
				return
			}

			reqBody := reqBodyOrRef.RequestBody
			if !assert.NotNil(t, reqBody) {
				return
			}

			content := reqBody.Content
			if !assert.Len(t, content, 1) {
				return
			}
			if !assert.Contains(t, content, JsonContent{}.ContentType()) {
				return
			}

			schemaOrRef := content[JsonContent{}.ContentType()].Schema
			if !assert.NotNil(t, schemaOrRef) {
				return
			}

			schemaRef := schemaOrRef.SchemaReference
			if !assert.NotNil(t, schemaRef) {
				return
			}
			_, schemaRefName := path.Split(schemaRef.Ref)

			comps := spec.Components
			if !assert.NotNil(t, comps) {
				return
			}

			schemas := comps.Schemas
			if !assert.NotNil(t, schemas) {
				return
			}

			schemaOrRefValues := schemas.MapOfSchemaOrRefValues
			if !assert.Len(t, schemaOrRefValues, 1) {
				return
			}
			if !assert.Contains(t, schemaOrRefValues, schemaRefName) {
				return
			}

			schema := schemaOrRefValues[schemaRefName].Schema
			if !assert.NotNil(t, schema) {
				return
			}

			props := schema.Properties
			if !assert.Len(t, props, 1) {
				return
			}
			if !assert.Contains(t, props, "value") {
				return
			}
		})
	})

	t.Run("will set response body type", func(t *testing.T) {
		t.Run("if the response type implements ContentTyper interface", func(t *testing.T) {
			method := strings.ToLower(http.MethodPost)
			pattern := "/"

			e := New(
				method,
				pattern,
				HandlerFunc[Empty, JsonContent](func(_ context.Context, _ Empty) (JsonContent, error) {
					return JsonContent{}, nil
				}),
			)

			refSpec := &openapi3.Spec{
				Openapi: "3.0.3",
			}
			e.OpenApi(refSpec)

			b, err := json.Marshal(refSpec)
			if !assert.Nil(t, err) {
				return
			}

			var spec openapi3.Spec
			err = json.Unmarshal(b, &spec)
			if !assert.Nil(t, err) {
				return
			}

			pathItems := spec.Paths.MapOfPathItemValues
			if !assert.Len(t, pathItems, 1) {
				return
			}
			if !assert.Contains(t, pathItems, pattern) {
				return
			}

			ops := pathItems[pattern].MapOfOperationValues
			if !assert.Len(t, ops, 1) {
				return
			}
			if !assert.Contains(t, ops, method) {
				return
			}

			respOrRefValues := ops[method].Responses.MapOfResponseOrRefValues
			if !assert.Len(t, respOrRefValues, 1) {
				return
			}
			if !assert.Contains(t, respOrRefValues, strconv.Itoa(DefaultStatusCode)) {
				return
			}

			resp := respOrRefValues[strconv.Itoa(DefaultStatusCode)].Response
			if !assert.NotNil(t, resp) {
				return
			}

			content := resp.Content
			if !assert.Len(t, content, 1) {
				return
			}
			if !assert.Contains(t, content, JsonContent{}.ContentType()) {
				return
			}

			schemaOrRef := content[JsonContent{}.ContentType()].Schema
			if !assert.NotNil(t, schemaOrRef) {
				return
			}

			schemaRef := schemaOrRef.SchemaReference
			if !assert.NotNil(t, schemaRef) {
				return
			}
			_, respRefName := path.Split(schemaRef.Ref)

			comps := spec.Components
			if !assert.NotNil(t, comps) {
				return
			}

			schemas := comps.Schemas
			if !assert.NotNil(t, schemas) {
				return
			}

			schemaOrRefValues := schemas.MapOfSchemaOrRefValues
			if !assert.Len(t, schemaOrRefValues, 1) {
				return
			}
			if !assert.Contains(t, schemaOrRefValues, respRefName) {
				return
			}

			schema := schemaOrRefValues[respRefName].Schema
			if !assert.NotNil(t, schema) {
				return
			}

			props := schema.Properties
			if !assert.Len(t, props, 1) {
				return
			}
			if !assert.Contains(t, props, "value") {
				return
			}
		})
	})

	t.Run("will set a empty response body", func(t *testing.T) {
		t.Run("if the response type does not implement ContentTyper", func(t *testing.T) {
			method := strings.ToLower(http.MethodPost)
			pattern := "/"

			e := New(
				method,
				pattern,
				HandlerFunc[Empty, Empty](func(_ context.Context, _ Empty) (Empty, error) {
					return Empty{}, nil
				}),
			)

			refSpec := &openapi3.Spec{
				Openapi: "3.0.3",
			}
			e.OpenApi(refSpec)

			b, err := json.Marshal(refSpec)
			if !assert.Nil(t, err) {
				return
			}

			var spec openapi3.Spec
			err = json.Unmarshal(b, &spec)
			if !assert.Nil(t, err) {
				return
			}

			pathItems := spec.Paths.MapOfPathItemValues
			if !assert.Len(t, pathItems, 1) {
				return
			}
			if !assert.Contains(t, pathItems, pattern) {
				return
			}

			ops := pathItems[pattern].MapOfOperationValues
			if !assert.Len(t, ops, 1) {
				return
			}
			if !assert.Contains(t, ops, method) {
				return
			}

			respOrRefValues := ops[method].Responses.MapOfResponseOrRefValues
			if !assert.Len(t, respOrRefValues, 1) {
				return
			}
			if !assert.Contains(t, respOrRefValues, strconv.Itoa(DefaultStatusCode)) {
				return
			}

			resp := respOrRefValues[strconv.Itoa(DefaultStatusCode)].Response
			if !assert.NotNil(t, resp) {
				return
			}

			content := resp.Content
			if !assert.Len(t, content, 0) {
				return
			}
		})

		t.Run("if the Returns option is used with a http status code", func(t *testing.T) {
			method := strings.ToLower(http.MethodPost)
			pattern := "/"
			statusCode := http.StatusBadRequest

			e := New(
				method,
				pattern,
				HandlerFunc[Empty, Empty](func(_ context.Context, _ Empty) (Empty, error) {
					return Empty{}, nil
				}),
				Returns(statusCode),
			)

			refSpec := &openapi3.Spec{
				Openapi: "3.0.3",
			}
			e.OpenApi(refSpec)

			b, err := json.Marshal(refSpec)
			if !assert.Nil(t, err) {
				return
			}

			var spec openapi3.Spec
			err = json.Unmarshal(b, &spec)
			if !assert.Nil(t, err) {
				return
			}

			pathItems := spec.Paths.MapOfPathItemValues
			if !assert.Len(t, pathItems, 1) {
				return
			}
			if !assert.Contains(t, pathItems, pattern) {
				return
			}

			ops := pathItems[pattern].MapOfOperationValues
			if !assert.Len(t, ops, 1) {
				return
			}
			if !assert.Contains(t, ops, method) {
				return
			}

			respOrRefValues := ops[method].Responses.MapOfResponseOrRefValues
			if !assert.Len(t, respOrRefValues, 2) {
				return
			}
			if !assert.Contains(t, respOrRefValues, strconv.Itoa(DefaultStatusCode)) {
				return
			}
			if !assert.Contains(t, respOrRefValues, strconv.Itoa(statusCode)) {
				return
			}

			defaultResp := respOrRefValues[strconv.Itoa(DefaultStatusCode)].Response
			if !assert.NotNil(t, defaultResp) {
				return
			}
			if !assert.Len(t, defaultResp.Content, 0) {
				return
			}

			resp := respOrRefValues[strconv.Itoa(statusCode)].Response
			if !assert.NotNil(t, resp) {
				return
			}
			if !assert.Len(t, resp.Content, 0) {
				return
			}
		})
	})

	t.Run("will override default response status code", func(t *testing.T) {
		t.Run("if DefaultStatusCode option is used", func(t *testing.T) {
			method := strings.ToLower(http.MethodPost)
			pattern := "/"

			statusCode := http.StatusCreated
			if !assert.NotEqual(t, statusCode, DefaultStatusCode) {
				return
			}

			e := New(
				method,
				pattern,
				HandlerFunc[Empty, Empty](func(_ context.Context, _ Empty) (Empty, error) {
					return Empty{}, nil
				}),
				StatusCode(statusCode),
			)

			refSpec := &openapi3.Spec{
				Openapi: "3.0.3",
			}
			e.OpenApi(refSpec)

			b, err := json.Marshal(refSpec)
			if !assert.Nil(t, err) {
				return
			}

			var spec openapi3.Spec
			err = json.Unmarshal(b, &spec)
			if !assert.Nil(t, err) {
				return
			}

			pathItems := spec.Paths.MapOfPathItemValues
			if !assert.Len(t, pathItems, 1) {
				return
			}
			if !assert.Contains(t, pathItems, pattern) {
				return
			}

			ops := pathItems[pattern].MapOfOperationValues
			if !assert.Len(t, ops, 1) {
				return
			}
			if !assert.Contains(t, ops, method) {
				return
			}

			respOrRefValues := ops[method].Responses.MapOfResponseOrRefValues
			if !assert.Len(t, respOrRefValues, 1) {
				return
			}
			if !assert.Contains(t, respOrRefValues, strconv.Itoa(statusCode)) {
				return
			}

			resp := respOrRefValues[strconv.Itoa(statusCode)].Response
			if !assert.NotNil(t, resp) {
				return
			}

			content := resp.Content
			if !assert.Len(t, content, 0) {
				return
			}
		})
	})
}
