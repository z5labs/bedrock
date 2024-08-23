// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package endpoint

import (
	"bytes"
	"context"
	"encoding"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/z5labs/bedrock/pkg/ptr"

	"github.com/swaggest/jsonschema-go"
	"github.com/swaggest/openapi-go/openapi3"
)

// Empty
type Empty struct{}

// Handler
type Handler[Req, Resp any] interface {
	Handle(context.Context, Req) (Resp, error)
}

// HandlerFunc
type HandlerFunc[Req, Resp any] func(context.Context, Req) (Resp, error)

// Handle implements the [Handler] interface.
func (f HandlerFunc[Req, Resp]) Handle(ctx context.Context, req Req) (Resp, error) {
	return f(ctx, req)
}

// ErrorHandler
type ErrorHandler interface {
	HandleError(http.ResponseWriter, error)
}

type options struct {
	method  string
	pattern string

	pathParams   map[PathParam]struct{}
	headerParams map[Header]struct{}
	queryParams  map[QueryParam]struct{}

	defaultStatusCode int
	validators        []func(*http.Request) error
	errHandler        ErrorHandler

	openapi *openapi3.Operation
}

// Option
type Option func(*options)

// Operation
type Operation[Req, Resp any] struct {
	validators []func(*http.Request) error
	injectors  []injector

	statusCode int
	handler    Handler[Req, Resp]

	errHandler ErrorHandler

	openapi *openapi3.Operation
}

const DefaultStatusCode = http.StatusOK

// StatusCode
func StatusCode(statusCode int) Option {
	return func(ho *options) {
		ho.defaultStatusCode = statusCode
	}
}

// PathParam
type PathParam struct {
	Name     string
	Pattern  string
	Required bool
}

// PathParams
func PathParams(ps ...PathParam) Option {
	return func(o *options) {
		for _, p := range ps {
			o.pathParams[p] = struct{}{}

			o.openapi.Parameters = append(o.openapi.Parameters, openapi3.ParameterOrRef{
				Parameter: &openapi3.Parameter{
					In:       openapi3.ParameterInPath,
					Name:     p.Name,
					Required: ptr.Ref(p.Required),
					Schema: &openapi3.SchemaOrRef{
						Schema: &openapi3.Schema{
							Type:    ptr.Ref(openapi3.SchemaTypeString),
							Pattern: ptr.Ref(p.Pattern),
						},
					},
				},
			})
		}
	}
}

// Header
type Header struct {
	Name     string
	Pattern  string
	Required bool
}

// Headers
func Headers(hs ...Header) Option {
	return func(o *options) {
		for _, h := range hs {
			o.headerParams[h] = struct{}{}

			o.openapi.Parameters = append(o.openapi.Parameters, openapi3.ParameterOrRef{
				Parameter: &openapi3.Parameter{
					In:       openapi3.ParameterInHeader,
					Name:     h.Name,
					Required: ptr.Ref(h.Required),
					Schema: &openapi3.SchemaOrRef{
						Schema: &openapi3.Schema{
							Type:    ptr.Ref(openapi3.SchemaTypeString),
							Pattern: ptr.Ref(h.Pattern),
						},
					},
				},
			})

			o.validators = append(o.validators, validateHeader(h))
		}
	}
}

// QueryParam
type QueryParam struct {
	Name     string
	Pattern  string
	Required bool
}

// QueryParams
func QueryParams(qps ...QueryParam) Option {
	return func(o *options) {
		for _, qp := range qps {
			o.queryParams[qp] = struct{}{}

			o.openapi.Parameters = append(o.openapi.Parameters, openapi3.ParameterOrRef{
				Parameter: &openapi3.Parameter{
					In:       openapi3.ParameterInQuery,
					Name:     qp.Name,
					Required: ptr.Ref(qp.Required),
					Schema: &openapi3.SchemaOrRef{
						Schema: &openapi3.Schema{
							Type:    ptr.Ref(openapi3.SchemaTypeString),
							Pattern: ptr.Ref(qp.Pattern),
						},
					},
				},
			})

			o.validators = append(o.validators, validateQueryParam(qp))
		}
	}
}

// ContentTyper
type ContentTyper interface {
	ContentType() string
}

// Accepts
func Accepts[Req any]() Option {
	return func(o *options) {
		contentType := ""

		var req Req
		if ct, ok := any(req).(ContentTyper); ok {
			contentType = ct.ContentType()

			o.validators = append(o.validators, validateHeader(Header{
				Name:     "Content-Type",
				Pattern:  fmt.Sprintf("^%s$", contentType),
				Required: true,
			}))
		}

		var reflector jsonschema.Reflector
		schema, err := reflector.Reflect(req)
		if err != nil {
			panic(err)
		}

		var schemaOrRef openapi3.SchemaOrRef
		schemaOrRef.FromJSONSchema(schema.ToSchemaOrBool())

		o.openapi.RequestBody = &openapi3.RequestBodyOrRef{
			RequestBody: &openapi3.RequestBody{
				Required: ptr.Ref(true),
				Content: map[string]openapi3.MediaType{
					contentType: {
						Schema: &schemaOrRef,
					},
				},
			},
		}
	}
}

// Returns
func Returns(status int) Option {
	return func(o *options) {
		o.openapi.Responses.MapOfResponseOrRefValues[strconv.Itoa(status)] = openapi3.ResponseOrRef{
			Response: &openapi3.Response{},
		}
	}
}

// ReturnsWith
func ReturnsWith[Resp any](status int) Option {
	return func(o *options) {
		var resp Resp
		ct, ok := any(resp).(ContentTyper)
		if !ok {
			o.openapi.Responses.MapOfResponseOrRefValues[strconv.Itoa(status)] = openapi3.ResponseOrRef{
				Response: &openapi3.Response{},
			}
			return
		}

		var reflector jsonschema.Reflector
		schema, err := reflector.Reflect(resp)
		if err != nil {
			panic(err)
		}

		var schemaOrRef openapi3.SchemaOrRef
		schemaOrRef.FromJSONSchema(schema.ToSchemaOrBool())

		o.openapi.Responses.MapOfResponseOrRefValues[strconv.Itoa(status)] = openapi3.ResponseOrRef{
			Response: &openapi3.Response{
				Content: map[string]openapi3.MediaType{
					ct.ContentType(): {
						Schema: &schemaOrRef,
					},
				},
			},
		}
	}
}

// OnError
func OnError(eh ErrorHandler) Option {
	return func(o *options) {
		o.errHandler = eh
	}
}

type errorHandlerFunc func(http.ResponseWriter, error)

func (f errorHandlerFunc) HandleError(w http.ResponseWriter, err error) {
	f(w, err)
}

// DefaultErrorStatusCode
const DefaultErrorStatusCode = http.StatusInternalServerError

// New initializes an Endpoint.
func New[Req, Resp any](handler Handler[Req, Resp], opts ...Option) *Operation[Req, Resp] {
	o := &options{
		defaultStatusCode: DefaultStatusCode,
		pathParams:        make(map[PathParam]struct{}),
		headerParams:      make(map[Header]struct{}),
		queryParams:       make(map[QueryParam]struct{}),
		errHandler: errorHandlerFunc(func(w http.ResponseWriter, err error) {
			w.WriteHeader(DefaultErrorStatusCode)
		}),
		openapi: &openapi3.Operation{
			Responses: openapi3.Responses{
				MapOfResponseOrRefValues: make(map[string]openapi3.ResponseOrRef),
			},
		},
	}

	for _, opt := range withBuiltinOptions[Req, Resp](opts...) {
		opt(o)
	}

	return &Operation[Req, Resp]{
		injectors:  initInjectors(o),
		validators: o.validators,
		statusCode: o.defaultStatusCode,
		handler:    handler,
		errHandler: o.errHandler,
		openapi:    o.openapi,
	}
}

func withBuiltinOptions[Req, Resp any](opts ...Option) []Option {
	var req Req
	if _, ok := any(req).(ContentTyper); ok {
		opts = append(opts, Accepts[Req]())
	}

	var resp Resp
	if _, ok := any(resp).(ContentTyper); ok {
		opts = append(opts, func(o *options) {
			ReturnsWith[Resp](o.defaultStatusCode)(o)
		})
	} else {
		opts = append(opts, func(o *options) {
			Returns(o.defaultStatusCode)(o)
		})
	}

	return opts
}

func initInjectors(o *options) []injector {
	injectors := []injector{injectResponseHeaders}
	for p := range o.pathParams {
		injectors = append(injectors, injectPathParam(p.Name))
	}
	if len(o.headerParams) > 0 {
		injectors = append(injectors, injectHeaders)
	}
	if len(o.queryParams) > 0 {
		injectors = append(injectors, injectQueryParams)
	}
	return injectors
}

func (op *Operation[Req, Resp]) OpenApi() *openapi3.Operation {
	return op.openapi
}

// ServeHTTP implements the [http.Handler] interface.
func (op *Operation[Req, Resp]) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := inject(r.Context(), w, r, op.injectors...)

	err := validateRequest(r, op.validators...)
	if err != nil {
		op.handleError(w, r, err)
		return
	}

	var req Req
	err = unmarshal(r.Body, &req)
	if err != nil {
		op.handleError(w, r, err)
		return
	}

	err = validate(req)
	if err != nil {
		op.handleError(w, r, err)
		return
	}

	resp, err := op.handler.Handle(ctx, req)
	if err != nil {
		op.handleError(w, r, err)
		return
	}

	bm, ok := any(resp).(encoding.BinaryMarshaler)
	if !ok {
		w.WriteHeader(op.statusCode)
		return
	}

	b, err := bm.MarshalBinary()
	if err != nil {
		op.handleError(w, r, err)
		return
	}

	if ct, ok := any(resp).(ContentTyper); ok {
		w.Header().Set("Content-Type", ct.ContentType())
	}

	w.WriteHeader(op.statusCode)
	_, err = io.Copy(w, bytes.NewReader(b))
	if err != nil {
		op.handleError(w, r, err)
		return
	}
}

func (op *Operation[Req, Resp]) handleError(w http.ResponseWriter, r *http.Request, err error) {
	if h, ok := err.(http.Handler); ok {
		h.ServeHTTP(w, r)
		return
	}

	op.errHandler.HandleError(w, err)
}

func unmarshal[Req any](r io.ReadCloser, req *Req) error {
	switch x := any(req).(type) {
	case encoding.BinaryUnmarshaler:
		defer func() {
			_ = r.Close()
		}()

		b, err := io.ReadAll(r)
		if err != nil {
			return err
		}

		return x.UnmarshalBinary(b)
	case io.ReaderFrom:
		_, err := x.ReadFrom(r)
		return err
	default:
		return nil
	}
}

// Validator
type Validator interface {
	Validate() error
}

func validate[Req any](req Req) error {
	if v, ok := any(req).(Validator); ok {
		return v.Validate()
	}
	return nil
}
