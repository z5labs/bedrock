// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"path"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/swaggest/openapi-go/openapi3"
	"github.com/z5labs/bedrock/rest/endpoint"
	"golang.org/x/sync/errgroup"
)

type JsonContent struct {
	Value string `json:"value"`
}

func (JsonContent) ContentType() string {
	return "application/json"
}

func fetchSpec(addr net.Addr, specCh chan<- openapi3.Spec) error {
	defer close(specCh)

	resp, err := http.Get(fmt.Sprintf("http://%s/openapi.json", addr))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var spec openapi3.Spec
	err = json.Unmarshal(b, &spec)
	if err != nil {
		return err
	}
	specCh <- spec
	return nil
}

func TestOpenApi(t *testing.T) {
	t.Run("will set request body type", func(t *testing.T) {
		t.Run("if the request type implements ContentTyper interface", func(t *testing.T) {
			addrCh := make(chan net.Addr)
			method := strings.ToLower(http.MethodPost)
			pattern := "/"

			app := NewApp(
				listenOnRandomPort(addrCh),
				Endpoint(
					endpoint.New(
						method,
						pattern,
						endpoint.HandlerFunc[JsonContent, endpoint.Empty](func(_ context.Context, _ JsonContent) (endpoint.Empty, error) {
							return endpoint.Empty{}, nil
						}),
					),
				),
			)

			ctx, cancel := context.WithCancel(context.Background())

			eg, egctx := errgroup.WithContext(ctx)
			eg.Go(func() error {
				return app.Run(egctx)
			})

			specCh := make(chan openapi3.Spec, 1)
			eg.Go(func() error {
				defer cancel()
				return fetchSpec(<-addrCh, specCh)
			})

			err := eg.Wait()
			if !assert.Nil(t, err) {
				return
			}

			spec := <-specCh

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
			addrCh := make(chan net.Addr)
			method := strings.ToLower(http.MethodPost)
			pattern := "/"

			app := NewApp(
				listenOnRandomPort(addrCh),
				Endpoint(
					endpoint.New(
						method,
						pattern,
						endpoint.HandlerFunc[endpoint.Empty, JsonContent](func(_ context.Context, _ endpoint.Empty) (JsonContent, error) {
							return JsonContent{}, nil
						}),
					),
				),
			)

			ctx, cancel := context.WithCancel(context.Background())

			eg, egctx := errgroup.WithContext(ctx)
			eg.Go(func() error {
				return app.Run(egctx)
			})

			specCh := make(chan openapi3.Spec, 1)
			eg.Go(func() error {
				defer cancel()
				return fetchSpec(<-addrCh, specCh)
			})

			err := eg.Wait()
			if !assert.Nil(t, err) {
				return
			}

			spec := <-specCh

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
			if !assert.Contains(t, respOrRefValues, strconv.Itoa(endpoint.DefaultStatusCode)) {
				return
			}

			resp := respOrRefValues[strconv.Itoa(endpoint.DefaultStatusCode)].Response
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
			addrCh := make(chan net.Addr)
			method := strings.ToLower(http.MethodPost)
			pattern := "/"

			app := NewApp(
				listenOnRandomPort(addrCh),
				Endpoint(
					endpoint.New(
						method,
						pattern,
						endpoint.HandlerFunc[endpoint.Empty, endpoint.Empty](func(_ context.Context, _ endpoint.Empty) (endpoint.Empty, error) {
							return endpoint.Empty{}, nil
						}),
					),
				),
			)

			ctx, cancel := context.WithCancel(context.Background())

			eg, egctx := errgroup.WithContext(ctx)
			eg.Go(func() error {
				return app.Run(egctx)
			})

			specCh := make(chan openapi3.Spec, 1)
			eg.Go(func() error {
				defer cancel()
				return fetchSpec(<-addrCh, specCh)
			})

			err := eg.Wait()
			if !assert.Nil(t, err) {
				return
			}

			spec := <-specCh

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
			if !assert.Contains(t, respOrRefValues, strconv.Itoa(endpoint.DefaultStatusCode)) {
				return
			}

			resp := respOrRefValues[strconv.Itoa(endpoint.DefaultStatusCode)].Response
			if !assert.NotNil(t, resp) {
				return
			}

			content := resp.Content
			if !assert.Len(t, content, 0) {
				return
			}
		})

		t.Run("if the Returns option is used with a http status code", func(t *testing.T) {
			addrCh := make(chan net.Addr)
			method := strings.ToLower(http.MethodPost)
			pattern := "/"
			statusCode := http.StatusBadRequest

			app := NewApp(
				listenOnRandomPort(addrCh),
				Endpoint(
					endpoint.New(
						method,
						pattern,
						endpoint.HandlerFunc[endpoint.Empty, endpoint.Empty](func(_ context.Context, _ endpoint.Empty) (endpoint.Empty, error) {
							return endpoint.Empty{}, nil
						}),
						endpoint.Returns(statusCode),
					),
				),
			)

			ctx, cancel := context.WithCancel(context.Background())

			eg, egctx := errgroup.WithContext(ctx)
			eg.Go(func() error {
				return app.Run(egctx)
			})

			specCh := make(chan openapi3.Spec, 1)
			eg.Go(func() error {
				defer cancel()
				return fetchSpec(<-addrCh, specCh)
			})

			err := eg.Wait()
			if !assert.Nil(t, err) {
				return
			}

			spec := <-specCh

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
			if !assert.Contains(t, respOrRefValues, strconv.Itoa(endpoint.DefaultStatusCode)) {
				return
			}
			if !assert.Contains(t, respOrRefValues, strconv.Itoa(statusCode)) {
				return
			}

			defaultResp := respOrRefValues[strconv.Itoa(endpoint.DefaultStatusCode)].Response
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
			addrCh := make(chan net.Addr)
			method := strings.ToLower(http.MethodPost)
			pattern := "/"

			statusCode := http.StatusCreated
			if !assert.NotEqual(t, statusCode, endpoint.DefaultStatusCode) {
				return
			}

			app := NewApp(
				listenOnRandomPort(addrCh),
				Endpoint(
					endpoint.New(
						method,
						pattern,
						endpoint.HandlerFunc[endpoint.Empty, endpoint.Empty](func(_ context.Context, _ endpoint.Empty) (endpoint.Empty, error) {
							return endpoint.Empty{}, nil
						}),
						endpoint.StatusCode(statusCode),
					),
				),
			)

			ctx, cancel := context.WithCancel(context.Background())

			eg, egctx := errgroup.WithContext(ctx)
			eg.Go(func() error {
				return app.Run(egctx)
			})

			specCh := make(chan openapi3.Spec, 1)
			eg.Go(func() error {
				defer cancel()
				return fetchSpec(<-addrCh, specCh)
			})

			err := eg.Wait()
			if !assert.Nil(t, err) {
				return
			}

			spec := <-specCh

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
