// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package endpoint

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/z5labs/bedrock/pkg/ptr"

	"github.com/stretchr/testify/assert"
	"github.com/swaggest/openapi-go/openapi3"
)

func TestEndpoint_OpenApi(t *testing.T) {
	t.Run("will required path parameter", func(t *testing.T) {
		t.Run("if a http.ServeMux path parameter pattern is used", func(t *testing.T) {
			e := NewOperation(
				noopHandler[Empty, Empty]{},
				PathParams(PathParam{
					Name:     "id",
					Required: true,
				}),
			)

			b, err := json.Marshal(e.OpenApi())
			if !assert.Nil(t, err) {
				return
			}

			var op openapi3.Operation
			err = json.Unmarshal(b, &op)
			if !assert.Nil(t, err) {
				return
			}

			params := op.Parameters
			if !assert.Len(t, params, 1) {
				return
			}

			param := params[0].Parameter
			if !assert.NotNil(t, param) {
				return
			}
			if !assert.Equal(t, openapi3.ParameterInPath, param.In) {
				return
			}
			if !assert.Equal(t, "id", param.Name) {
				return
			}
			if !assert.True(t, ptr.Deref(param.Required)) {
				return
			}
		})
	})

	t.Run("will set non-required header parameter", func(t *testing.T) {
		t.Run("if a header is provided with the Headers option", func(t *testing.T) {
			header := Header{
				Name:     "MyHeader",
				Required: true,
			}

			e := NewOperation(
				noopHandler[Empty, Empty]{},
				Headers(header),
			)

			b, err := json.Marshal(e.OpenApi())
			if !assert.Nil(t, err) {
				return
			}

			var op openapi3.Operation
			err = json.Unmarshal(b, &op)
			if !assert.Nil(t, err) {
				return
			}

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
			header := Header{
				Name:     "MyHeader",
				Required: true,
			}

			e := NewOperation(
				noopHandler[Empty, Empty]{},
				Headers(header),
			)

			b, err := json.Marshal(e.OpenApi())
			if !assert.Nil(t, err) {
				return
			}

			var op openapi3.Operation
			err = json.Unmarshal(b, &op)
			if !assert.Nil(t, err) {
				return
			}

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

	t.Run("will set non-required query param", func(t *testing.T) {
		t.Run("if a query param is provided with the QueryParams option", func(t *testing.T) {
			queryParam := QueryParam{
				Name: "myparam",
			}

			e := NewOperation(
				noopHandler[Empty, Empty]{},
				QueryParams(queryParam),
			)

			b, err := json.Marshal(e.OpenApi())
			if !assert.Nil(t, err) {
				return
			}

			var op openapi3.Operation
			err = json.Unmarshal(b, &op)
			if !assert.Nil(t, err) {
				return
			}

			params := op.Parameters
			if !assert.Len(t, params, 1) {
				return
			}

			param := params[0].Parameter
			if !assert.NotNil(t, param) {
				return
			}
			if !assert.Equal(t, openapi3.ParameterInQuery, param.In) {
				return
			}
			if !assert.Equal(t, queryParam.Name, param.Name) {
				return
			}
			if !assert.Equal(t, queryParam.Required, ptr.Deref(param.Required)) {
				return
			}
		})
	})

	t.Run("will set required query param", func(t *testing.T) {
		t.Run("if a query param is provided with the QueryParams option", func(t *testing.T) {
			queryParam := QueryParam{
				Name:     "myparam",
				Required: true,
			}

			e := NewOperation(
				noopHandler[Empty, Empty]{},
				QueryParams(queryParam),
			)

			b, err := json.Marshal(e.OpenApi())
			if !assert.Nil(t, err) {
				return
			}

			var op openapi3.Operation
			err = json.Unmarshal(b, &op)
			if !assert.Nil(t, err) {
				return
			}

			params := op.Parameters
			if !assert.Len(t, params, 1) {
				return
			}

			param := params[0].Parameter
			if !assert.NotNil(t, param) {
				return
			}
			if !assert.Equal(t, openapi3.ParameterInQuery, param.In) {
				return
			}
			if !assert.Equal(t, queryParam.Name, param.Name) {
				return
			}
			if !assert.Equal(t, queryParam.Required, ptr.Deref(param.Required)) {
				return
			}
		})
	})

	t.Run("will set request body type", func(t *testing.T) {
		t.Run("if the request type implements ContentTyper interface", func(t *testing.T) {
			type jsonContent struct {
				Value string `json:"value"`
			}

			e := NewOperation(
				ConsumesJson(HandlerFunc[jsonContent, Empty](func(_ context.Context, _ *jsonContent) (*Empty, error) {
					return &Empty{}, nil
				})),
			)

			b, err := json.Marshal(e.OpenApi())
			if !assert.Nil(t, err) {
				return
			}

			var op openapi3.Operation
			err = json.Unmarshal(b, &op)
			if !assert.Nil(t, err) {
				return
			}

			reqBodyOrRef := op.RequestBody
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

			ct := (&JsonResponse[jsonContent]{}).ContentType()
			if !assert.Contains(t, content, ct) {
				return
			}

			schemaOrRef := content[ct].Schema
			if !assert.NotNil(t, schemaOrRef) {
				return
			}

			schema := schemaOrRef.Schema
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
			type jsonContent struct {
				Value string `json:"value"`
			}

			e := NewOperation(
				ProducesJson(HandlerFunc[Empty, jsonContent](func(_ context.Context, _ *Empty) (*jsonContent, error) {
					return &jsonContent{}, nil
				})),
			)

			b, err := json.Marshal(e.OpenApi())
			if !assert.Nil(t, err) {
				return
			}

			var op openapi3.Operation
			err = json.Unmarshal(b, &op)
			if !assert.Nil(t, err) {
				return
			}

			respOrRefValues := op.Responses.MapOfResponseOrRefValues
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

			ct := (&JsonResponse[jsonContent]{}).ContentType()
			if !assert.Contains(t, content, ct) {
				return
			}

			schemaOrRef := content[ct].Schema
			if !assert.NotNil(t, schemaOrRef) {
				return
			}

			schema := schemaOrRef.Schema
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
			e := NewOperation(
				noopHandler[Empty, Empty]{},
			)

			b, err := json.Marshal(e.OpenApi())
			if !assert.Nil(t, err) {
				return
			}

			var op openapi3.Operation
			err = json.Unmarshal(b, &op)
			if !assert.Nil(t, err) {
				return
			}

			respOrRefValues := op.Responses.MapOfResponseOrRefValues
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
			statusCode := http.StatusBadRequest

			e := NewOperation(
				noopHandler[Empty, Empty]{},
				Returns(statusCode),
			)

			b, err := json.Marshal(e.OpenApi())
			if !assert.Nil(t, err) {
				return
			}

			var op openapi3.Operation
			err = json.Unmarshal(b, &op)
			if !assert.Nil(t, err) {
				return
			}

			respOrRefValues := op.Responses.MapOfResponseOrRefValues
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
			statusCode := http.StatusCreated
			if !assert.NotEqual(t, statusCode, DefaultStatusCode) {
				return
			}

			e := NewOperation(
				noopHandler[Empty, Empty]{},
				StatusCode(statusCode),
			)

			b, err := json.Marshal(e.OpenApi())
			if !assert.Nil(t, err) {
				return
			}

			var op openapi3.Operation
			err = json.Unmarshal(b, &op)
			if !assert.Nil(t, err) {
				return
			}

			respOrRefValues := op.Responses.MapOfResponseOrRefValues
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
