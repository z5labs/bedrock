// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package endpoint

import (
	"github.com/swaggest/openapi-go/openapi3"
)

func setOpenApiSpec(o *options) func(*openapi3.Spec) {
	return compose(
		addSchemas(o.schemas),
		addParameters(o.method, o.pattern, o.pathParams...),
		addParameters(o.method, o.pattern, o.headers...),
		addParameters(o.method, o.pattern, o.queryParams...),
		addRequestBody(o.method, o.pattern, o.request),
		addResponses(o.method, o.pattern, o.responses),
	)
}

func compose(fs ...func(*openapi3.Spec)) func(*openapi3.Spec) {
	return func(s *openapi3.Spec) {
		for _, f := range fs {
			f(s)
		}
	}
}

func addSchemas(schemas map[string]*openapi3.Schema) func(*openapi3.Spec) {
	return func(s *openapi3.Spec) {
		if len(schemas) == 0 {
			return
		}

		if s.Components == nil {
			s.Components = new(openapi3.Components)
		}

		comps := s.Components
		if comps.Schemas == nil {
			comps.Schemas = new(openapi3.ComponentsSchemas)
		}

		compSchemas := comps.Schemas
		if compSchemas.MapOfSchemaOrRefValues == nil {
			compSchemas.MapOfSchemaOrRefValues = make(map[string]openapi3.SchemaOrRef, len(schemas))
		}

		for name, schema := range schemas {
			compSchemas.MapOfSchemaOrRefValues[name] = openapi3.SchemaOrRef{
				Schema: schema,
			}
		}
	}
}

func addParameters(method, pattern string, params ...*openapi3.Parameter) func(*openapi3.Spec) {
	return func(s *openapi3.Spec) {
		if len(params) == 0 {
			return
		}

		if s.Paths.MapOfPathItemValues == nil {
			s.Paths.MapOfPathItemValues = make(map[string]openapi3.PathItem)
		}

		pathItemVals := s.Paths.MapOfPathItemValues
		pathItem, ok := pathItemVals[pattern]
		if !ok {
			pathItem = openapi3.PathItem{
				MapOfOperationValues: make(map[string]openapi3.Operation),
			}
		}

		opVals := pathItem.MapOfOperationValues
		opVal, ok := opVals[method]
		if !ok {
			opVal = openapi3.Operation{}
		}

		for _, param := range params {
			opVal.Parameters = append(opVal.Parameters, openapi3.ParameterOrRef{
				Parameter: param,
			})
		}

		opVals[method] = opVal
		pathItemVals[pattern] = pathItem
	}
}

func addRequestBody(method, pattern string, reqBody *openapi3.RequestBody) func(*openapi3.Spec) {
	return func(s *openapi3.Spec) {
		if reqBody == nil {
			return
		}

		if s.Paths.MapOfPathItemValues == nil {
			s.Paths.MapOfPathItemValues = make(map[string]openapi3.PathItem)
		}

		pathItemVals := s.Paths.MapOfPathItemValues
		pathItem, ok := pathItemVals[pattern]
		if !ok {
			pathItem = openapi3.PathItem{
				MapOfOperationValues: make(map[string]openapi3.Operation),
			}
		}

		opVals := pathItem.MapOfOperationValues
		opVal, ok := opVals[method]
		if !ok {
			opVal = openapi3.Operation{}
		}

		opVal.RequestBody = &openapi3.RequestBodyOrRef{
			RequestBody: reqBody,
		}

		opVals[method] = opVal
		pathItemVals[pattern] = pathItem
	}
}

func addResponses(method, pattern string, responses *openapi3.Responses) func(*openapi3.Spec) {
	return func(s *openapi3.Spec) {
		if responses == nil {
			return
		}

		if s.Paths.MapOfPathItemValues == nil {
			s.Paths.MapOfPathItemValues = make(map[string]openapi3.PathItem)
		}

		pathItemVals := s.Paths.MapOfPathItemValues
		pathItem, ok := pathItemVals[pattern]
		if !ok {
			pathItem = openapi3.PathItem{
				MapOfOperationValues: make(map[string]openapi3.Operation),
			}
		}

		opVals := pathItem.MapOfOperationValues
		opVal, ok := opVals[method]
		if !ok {
			opVal = openapi3.Operation{}
		}

		opVal.Responses = *responses

		opVals[method] = opVal
		pathItemVals[pattern] = pathItem
	}
}
