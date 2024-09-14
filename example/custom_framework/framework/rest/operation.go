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

func NewOperation[Req, Resp any](h OperationHandler[Req, Resp], opts ...OperationOption) rest.Operation {
	opOpts := &operationConfig{}
	for _, opt := range opts {
		opt(opOpts)
	}

	var endpointOpts []endpoint.Option
	if len(opOpts.headers) > 0 {
		endpointOpts = append(endpointOpts, endpoint.Headers(opOpts.headers...))
	}

	return endpoint.NewOperation(h, endpointOpts...)
}
