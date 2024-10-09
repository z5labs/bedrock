// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package endpoint

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"

	"github.com/z5labs/bedrock/pkg/ptr"

	"github.com/swaggest/openapi-go/openapi3"
	"go.opentelemetry.io/otel"
)

// Handler defines an RPC inspired way of handling HTTP requests.
//
// Req and Resp can implement various interfaces which [Operation]
// uses to automate many tasks before and after calling your Handler.
// For example, [Operation] handles unmarshaling and marshaling the request (Req)
// and response (Resp) types automatically if they implement [encoding.BinaryUnmarshaler]
// and [encoding.BinaryMarshaler], respectively.
type Handler[Req, Resp any] interface {
	Handle(context.Context, *Req) (*Resp, error)
}

// HandlerFunc is an adapter type to allow the use of ordinary functions as [Handler]s.
type HandlerFunc[Req, Resp any] func(context.Context, *Req) (*Resp, error)

// Handle implements the [Handler] interface.
func (f HandlerFunc[Req, Resp]) Handle(ctx context.Context, req *Req) (*Resp, error) {
	return f(ctx, req)
}

// ErrorHandler	defines the behaviour taken by [Operation]
// when a [Handler] returns an [error].
type ErrorHandler interface {
	HandleError(context.Context, http.ResponseWriter, error)
}

type options struct {
	pathParams   map[PathParam]struct{}
	headerParams map[Header]struct{}
	queryParams  map[QueryParam]struct{}

	defaultStatusCode int
	validators        []func(*http.Request) error
	errHandler        ErrorHandler

	openapi openapi3.Operation
}

// Option configures a [Operation].
type Option func(*options)

// Operation is a RPC inspired [http.Handler] (aka endpoint) that also
// keeps track of the associated types and parameters
// in order to construct an OpenAPI operation definition.
type Operation[I, O any, Req Request[I], Resp Response[O]] struct {
	validators []func(*http.Request) error
	injectors  []injector

	statusCode   int
	handler      Handler[I, O]
	writeBufPool *sync.Pool

	errHandler ErrorHandler

	openapi openapi3.Operation
}

// DefaultStatusCode is the default HTTP status code returned
// by an [Operation] when the underlying [Handler] does not return an [error].
const DefaultStatusCode = http.StatusOK

// StatusCode will change the HTTP status code that is returned
// by an [Operation] when the underlying [Handler] does not return an [error].
func StatusCode(statusCode int) Option {
	return func(ho *options) {
		ho.defaultStatusCode = statusCode
	}
}

// PathParam defines a URL path parameter e.g. /book/{id} where id is the path param.
type PathParam struct {
	Name     string
	Pattern  string
	Required bool
}

// PathParams registers the [PathParam]s with the OpenAPI operation definition.
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

			o.validators = append(o.validators, validatePathParam(p))
		}
	}
}

// Header defines a HTTP header.
type Header struct {
	Name     string
	Pattern  string
	Required bool
}

// Headers registers the [Header]s with the OpenAPI operation definition.
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

// QueryParam defines a URL query parameter e.g. /book?id=123
type QueryParam struct {
	Name     string
	Pattern  string
	Required bool
}

// QueryParams registers the [QueryParam]s with the OpenAPI operation definition.
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

// ContentTyper is the interface which request and response types
// should implement in order to allow the [Operation] to automatically
// validate and set the "Content-Type" HTTP Header along with
// properly documenting the types in the OpenAPI operation definition.
type ContentTyper interface {
	ContentType() string
}

// OpenApiV3Schemaer
type OpenApiV3Schemaer interface {
	OpenApiV3Schema() (*openapi3.Schema, error)
}

// Accepts registers the Req type in the OpenAPI operation definition
// as a possible request to the [Operation].
func Accepts[I any, Req Request[I]]() Option {
	return func(o *options) {
		var i I
		var req Req = &i
		ct := req.ContentType()
		if len(ct) > 0 {
			o.validators = append(o.validators, validateHeader(Header{
				Name:     "Content-Type",
				Pattern:  fmt.Sprintf("^%s$", ct),
				Required: true,
			}))
		}

		schema, err := req.OpenApiV3Schema()
		if err != nil {
			panic(err)
		}

		var schemaOrRef openapi3.SchemaOrRef
		schemaOrRef.Schema = schema

		o.openapi.RequestBody = &openapi3.RequestBodyOrRef{
			RequestBody: &openapi3.RequestBody{
				Required: ptr.Ref(true),
				Content: map[string]openapi3.MediaType{
					ct: {
						Schema: &schemaOrRef,
					},
				},
			},
		}
	}
}

// Returns registers the status code in the OpenAPI operation
// definition as a possible response from the [Operation].
func Returns(status int) Option {
	return func(o *options) {
		o.openapi.Responses.MapOfResponseOrRefValues[strconv.Itoa(status)] = openapi3.ResponseOrRef{
			Response: &openapi3.Response{},
		}
	}
}

// ReturnsWith registers the Resp type and status code in the OpenAPI
// operation definition as a possible response from the [Operation].
func ReturnsWith[O any, Resp Response[O]](status int) Option {
	return func(opts *options) {
		var o O
		var resp Resp = &o
		ct := resp.ContentType()
		if len(ct) == 0 {
			opts.openapi.Responses.MapOfResponseOrRefValues[strconv.Itoa(status)] = openapi3.ResponseOrRef{
				Response: &openapi3.Response{},
			}
			return
		}

		schema, err := resp.OpenApiV3Schema()
		if err != nil {
			panic(err)
		}

		var schemaOrRef openapi3.SchemaOrRef
		schemaOrRef.Schema = schema

		opts.openapi.Responses.MapOfResponseOrRefValues[strconv.Itoa(status)] = openapi3.ResponseOrRef{
			Response: &openapi3.Response{
				Content: map[string]openapi3.MediaType{
					ct: {
						Schema: &schemaOrRef,
					},
				},
			},
		}
	}
}

// OnError registers the [ErrorHandler] with the [Operation]. Any
// [error]s returned by the underlying [Handler] will be passed to
// this [ErrorHandler].
func OnError(eh ErrorHandler) Option {
	return func(o *options) {
		o.errHandler = eh
	}
}

type errorHandlerFunc func(context.Context, http.ResponseWriter, error)

func (f errorHandlerFunc) HandleError(ctx context.Context, w http.ResponseWriter, err error) {
	f(ctx, w, err)
}

// DefaultErrorStatusCode is the default HTTP status code returned by
// an [Operation] if no [ErrorHandler] has been registered with the
// [OnError] option and the underlying [Handler] returns an [error].
const DefaultErrorStatusCode = http.StatusInternalServerError

// NewOperation initializes a Operation.
func NewOperation[I, O any, Req Request[I], Resp Response[O]](handler Handler[I, O], opts ...Option) *Operation[I, O, Req, Resp] {
	o := &options{
		defaultStatusCode: DefaultStatusCode,
		pathParams:        make(map[PathParam]struct{}),
		headerParams:      make(map[Header]struct{}),
		queryParams:       make(map[QueryParam]struct{}),
		errHandler: errorHandlerFunc(func(ctx context.Context, w http.ResponseWriter, err error) {
			w.WriteHeader(DefaultErrorStatusCode)
		}),
		openapi: openapi3.Operation{
			Responses: openapi3.Responses{
				MapOfResponseOrRefValues: make(map[string]openapi3.ResponseOrRef),
			},
		},
	}

	for _, opt := range withBuiltinOptions[I, O, Req, Resp](opts...) {
		opt(o)
	}

	return &Operation[I, O, Req, Resp]{
		injectors:  initInjectors(o),
		validators: o.validators,
		statusCode: o.defaultStatusCode,
		handler:    handler,
		writeBufPool: &sync.Pool{
			New: func() any {
				return new(bytes.Buffer)
			},
		},
		errHandler: o.errHandler,
		openapi:    o.openapi,
	}
}

func withBuiltinOptions[I, O any, Req Request[I], Resp Response[O]](opts ...Option) []Option {
	var i I
	var req Req = &i
	ct := req.ContentType()
	if len(ct) > 0 {
		opts = append(opts, Accepts[I, Req]())
	}

	var o O
	var resp Resp = &o
	ct = resp.ContentType()
	if len(ct) > 0 {
		opts = append(opts, func(o *options) {
			ReturnsWith[O, Resp](o.defaultStatusCode)(o)
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

// OpenApi returns the OpenAPI operation definition for this endpoint.
func (op *Operation[I, O, Req, Resp]) OpenApi() openapi3.Operation {
	return op.openapi
}

// ServeHTTP implements the [http.Handler] interface.
func (op *Operation[I, O, Req, Resp]) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	spanCtx, span := otel.Tracer("endpoint").Start(r.Context(), "Operation.ServeHTTP")
	defer span.End()

	ctx := inject(spanCtx, w, r, op.injectors...)

	err := validateRequest(ctx, r, op.validators...)
	if err != nil {
		op.handleError(ctx, w, err)
		return
	}

	var i I
	var req Req = &i
	err = readRequest(ctx, r, req)
	if err != nil {
		op.handleError(ctx, w, err)
		return
	}

	err = validate(ctx, req)
	if err != nil {
		op.handleError(ctx, w, err)
		return
	}

	resp, err := op.handler.Handle(ctx, req)
	if err != nil {
		op.handleError(ctx, w, err)
		return
	}
	if resp == nil {
		op.handleError(ctx, w, ErrNilHandlerResponse)
		return
	}

	err = op.writeResponse(ctx, w, resp)
	if err != nil {
		op.handleError(ctx, w, err)
		return
	}
}

func readRequest[Req RequestReader](ctx context.Context, r *http.Request, req Req) error {
	_, span := otel.Tracer("endpoint").Start(ctx, "readRequest")
	defer span.End()

	return req.ReadRequest(r)
}

func validate[Req Validator](ctx context.Context, req Req) error {
	_, span := otel.Tracer("endpoint").Start(ctx, "validate")
	defer span.End()

	err := req.Validate()
	span.RecordError(err)
	return err
}

// ErrNilHandlerResponse
var ErrNilHandlerResponse = errors.New("received nil for response that is expected to be in response body")

func (op *Operation[I, O, Req, Resp]) writeResponse(ctx context.Context, w http.ResponseWriter, resp Resp) error {
	_, span := otel.Tracer("endpoint").Start(ctx, "Operation.writeResponse")
	defer span.End()

	buf := op.getWriteBuf()
	defer op.putWriteBuf(buf)

	_, err := resp.WriteTo(buf)
	if err != nil {
		span.RecordError(err)
		return err
	}

	ct := resp.ContentType()
	if len(ct) > 0 {
		w.Header().Set("Content-Type", ct)
	}

	w.WriteHeader(op.statusCode)

	span.RecordError(err)
	_, err = io.Copy(w, buf)
	return err
}

func (op *Operation[I, O, Req, Resp]) handleError(ctx context.Context, w http.ResponseWriter, err error) {
	spanCtx, span := otel.Tracer("endpoint").Start(ctx, "Operation.handleError")
	defer span.End()

	op.errHandler.HandleError(spanCtx, w, err)
}

func (op *Operation[I, O, Req, Resp]) getWriteBuf() *bytes.Buffer {
	v := op.writeBufPool.Get()
	if v == nil {
		return new(bytes.Buffer)
	}

	buf, ok := v.(*bytes.Buffer)
	if !ok {
		return new(bytes.Buffer)
	}
	return buf
}

func (op *Operation[I, O, Req, Resp]) putWriteBuf(buf *bytes.Buffer) {
	buf.Reset()
	op.writeBufPool.Put(buf)
}
