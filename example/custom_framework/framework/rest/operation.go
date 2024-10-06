// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package rest

import (
	"github.com/z5labs/bedrock/rest"
	"github.com/z5labs/bedrock/rest/endpoint"
)

type Operation rest.Operation

type OperationHandler[Req, Resp any] endpoint.Handler[Req, Resp]

type operationConfig struct {
	headers []endpoint.Header
}

type OperationOption func(*operationConfig)

func Header(name string, required bool, pattern string) OperationOption {
	return func(oc *operationConfig) {
		oc.headers = append(oc.headers, endpoint.Header{
			Name:     name,
			Required: required,
			Pattern:  pattern,
		})
	}
}

func NewOperation[I, O any, Req endpoint.Request[I], Resp endpoint.Response[O]](h OperationHandler[I, O], opts ...OperationOption) rest.Operation {
	opOpts := &operationConfig{}
	for _, opt := range opts {
		opt(opOpts)
	}

	var endpointOpts []endpoint.Option
	if len(opOpts.headers) > 0 {
		endpointOpts = append(endpointOpts, endpoint.Headers(opOpts.headers...))
	}

	return endpoint.NewOperation[I, O, Req, Resp](h, endpointOpts...)
}
