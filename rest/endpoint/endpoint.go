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
	"reflect"
	"strconv"
	"strings"

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

	defaultStatusCode int
	validators        []func(*http.Request) error
	errHandler        ErrorHandler

	schemas     map[string]*openapi3.Schema
	pathParams  []*openapi3.Parameter
	headers     []*openapi3.Parameter
	queryParams []*openapi3.Parameter
	request     *openapi3.RequestBody
	responses   *openapi3.Responses
}

// Option
type Option func(*options)

// Endpoint
type Endpoint[Req, Resp any] struct {
	method  string
	pattern string

	validators []func(*http.Request) error
	injectors  []injector

	statusCode int
	handler    Handler[Req, Resp]

	errHandler ErrorHandler

	openapi func(*openapi3.Spec)
}

const DefaultStatusCode = http.StatusOK

// StatusCode
func StatusCode(statusCode int) Option {
	return func(ho *options) {
		ho.defaultStatusCode = statusCode
	}
}

type pathParam struct {
	name string
}

func parsePathParams(s string) []pathParam {
	var params []pathParam
	var found bool
	for {
		if len(s) == 0 {
			return params
		}

		_, s, found = strings.Cut(s, "{")
		if !found {
			return params
		}

		i := strings.IndexByte(s, '}')
		if i == -1 {
			return params
		}

		param := s[:i]
		s = s[i:]

		name := strings.TrimSuffix(param, ".")
		params = append(params, pathParam{
			name: name,
		})
	}
}

func pathParams(ps ...pathParam) Option {
	return func(o *options) {
		for _, p := range ps {
			o.pathParams = append(o.pathParams, &openapi3.Parameter{
				In:       openapi3.ParameterInPath,
				Name:     p.name,
				Required: ptr.Ref(true),
				Schema: &openapi3.SchemaOrRef{
					Schema: &openapi3.Schema{
						Type: ptr.Ref(openapi3.SchemaTypeString),
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
			o.headers = append(o.headers, &openapi3.Parameter{
				In:       openapi3.ParameterInHeader,
				Name:     h.Name,
				Required: ptr.Ref(h.Required),
				Schema: &openapi3.SchemaOrRef{
					Schema: &openapi3.Schema{
						Type: ptr.Ref(openapi3.SchemaTypeString),
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
			o.queryParams = append(o.queryParams, &openapi3.Parameter{
				In:       openapi3.ParameterInQuery,
				Name:     qp.Name,
				Required: ptr.Ref(qp.Required),
				Schema: &openapi3.SchemaOrRef{
					Schema: &openapi3.Schema{
						Type: ptr.Ref(openapi3.SchemaTypeString),
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
		if o.request == nil {
			o.request = new(openapi3.RequestBody)
		}

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

		typeName := reflect.TypeOf(req).Name()
		schemaRef := fmt.Sprintf("#/components/schemas/%s", typeName)
		o.schemas[typeName] = schemaOrRef.Schema

		o.request = &openapi3.RequestBody{
			Required: ptr.Ref(true),
			Content: map[string]openapi3.MediaType{
				contentType: {
					Schema: &openapi3.SchemaOrRef{
						SchemaReference: &openapi3.SchemaReference{
							Ref: schemaRef,
						},
					},
				},
			},
		}
	}
}

// Returns
func Returns(status int) Option {
	return func(o *options) {
		if o.responses == nil {
			o.responses = &openapi3.Responses{
				MapOfResponseOrRefValues: make(map[string]openapi3.ResponseOrRef),
			}
		}

		o.responses.MapOfResponseOrRefValues[strconv.Itoa(status)] = openapi3.ResponseOrRef{
			Response: &openapi3.Response{},
		}
	}
}

// ReturnsWith
func ReturnsWith[Resp any](status int) Option {
	return func(o *options) {
		if o.responses == nil {
			o.responses = &openapi3.Responses{
				MapOfResponseOrRefValues: make(map[string]openapi3.ResponseOrRef),
			}
		}

		contentType := ""

		var resp Resp
		if ct, ok := any(resp).(ContentTyper); ok {
			contentType = ct.ContentType()
		} else {
			o.responses.MapOfResponseOrRefValues[strconv.Itoa(status)] = openapi3.ResponseOrRef{
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

		typeName := reflect.TypeOf(resp).Name()
		schemaRef := fmt.Sprintf("#/components/schemas/%s", typeName)
		o.schemas[typeName] = schemaOrRef.Schema

		o.responses.MapOfResponseOrRefValues[strconv.Itoa(status)] = openapi3.ResponseOrRef{
			Response: &openapi3.Response{
				Content: map[string]openapi3.MediaType{
					contentType: {
						Schema: &openapi3.SchemaOrRef{
							SchemaReference: &openapi3.SchemaReference{
								Ref: schemaRef,
							},
						},
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

const DefaultErrorStatusCode = http.StatusInternalServerError

// New initializes an Endpoint.
func New[Req, Resp any](method string, pattern string, handler Handler[Req, Resp], opts ...Option) *Endpoint[Req, Resp] {
	parsedPathParams := parsePathParams(pattern)
	opts = append(opts, pathParams(parsedPathParams...))

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

	o := &options{
		method:            method,
		pattern:           pattern,
		defaultStatusCode: DefaultStatusCode,
		validators: []func(*http.Request) error{
			validateMethod(method),
		},
		errHandler: errorHandlerFunc(func(w http.ResponseWriter, err error) {
			w.WriteHeader(DefaultErrorStatusCode)
		}),
		schemas: make(map[string]*openapi3.Schema),
	}

	for _, opt := range opts {
		opt(o)
	}

	injectors := []injector{injectResponseHeaders}
	for _, p := range parsedPathParams {
		injectors = append(injectors, injectPathParam(p.name))
	}
	if len(o.headers) > 0 {
		injectors = append(injectors, injectHeaders)
	}
	if len(o.queryParams) > 0 {
		injectors = append(injectors, injectQueryParams)
	}

	return &Endpoint[Req, Resp]{
		method:     method,
		pattern:    pattern,
		injectors:  injectors,
		validators: o.validators,
		statusCode: o.defaultStatusCode,
		handler:    handler,
		errHandler: o.errHandler,
		openapi:    setOpenApiSpec(o),
	}
}

// Get returns an Endpoint configured for handling HTTP GET requests.
func Get[Req, Resp any](pattern string, handler Handler[Req, Resp], opts ...Option) *Endpoint[Req, Resp] {
	return New(http.MethodGet, pattern, handler, opts...)
}

// Post returns an Endpoint configured for handling HTTP POST requests.
func Post[Req, Resp any](pattern string, handler Handler[Req, Resp], opts ...Option) *Endpoint[Req, Resp] {
	return New(http.MethodPost, pattern, handler, opts...)
}

// Put returns an Endpoint configured for handling HTTP PUT requests.
func Put[Req, Resp any](pattern string, handler Handler[Req, Resp], opts ...Option) *Endpoint[Req, Resp] {
	return New(http.MethodPut, pattern, handler, opts...)
}

// Delete returns an Endpoint configured for handling HTTP DELETE requests.
func Delete[Req, Resp any](pattern string, handler Handler[Req, Resp], opts ...Option) *Endpoint[Req, Resp] {
	return New(http.MethodDelete, pattern, handler, opts...)
}

// Method returns the HTTP method which this endpoint
// is configured to handle requests for.
func (e *Endpoint[Req, Resp]) Method() string {
	return e.method
}

// Pattern returns HTTP path pattern for this endpoint.
func (e *Endpoint[Req, Resp]) Pattern() string {
	return e.pattern
}

// OpenApi allows the endpoint to register itself with an OpenAPI spec.
func (e *Endpoint[Req, Resp]) OpenApi(spec *openapi3.Spec) {
	e.openapi(spec)
}

// ServeHTTP implements the [http.Handler] interface.
func (e *Endpoint[Req, Resp]) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := inject(r.Context(), w, r, e.injectors...)

	err := validateRequest(r, e.validators...)
	if err != nil {
		e.handleError(w, r, err)
		return
	}

	var req Req
	err = unmarshal(r.Body, &req)
	if err != nil {
		e.handleError(w, r, err)
		return
	}

	err = validate(req)
	if err != nil {
		e.handleError(w, r, err)
		return
	}

	resp, err := e.handler.Handle(ctx, req)
	if err != nil {
		e.handleError(w, r, err)
		return
	}

	bm, ok := any(resp).(encoding.BinaryMarshaler)
	if !ok {
		w.WriteHeader(e.statusCode)
		return
	}

	b, err := bm.MarshalBinary()
	if err != nil {
		e.handleError(w, r, err)
		return
	}

	if ct, ok := any(resp).(ContentTyper); ok {
		w.Header().Set("Content-Type", ct.ContentType())
	}

	w.WriteHeader(e.statusCode)
	_, err = io.Copy(w, bytes.NewReader(b))
	if err != nil {
		e.handleError(w, r, err)
		return
	}
}

func (e *Endpoint[Req, Resp]) handleError(w http.ResponseWriter, r *http.Request, err error) {
	if h, ok := err.(http.Handler); ok {
		h.ServeHTTP(w, r)
		return
	}

	e.errHandler.HandleError(w, err)
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
