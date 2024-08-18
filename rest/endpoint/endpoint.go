// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package endpoint

import (
	"bytes"
	"context"
	"encoding"
	"errors"
	"io"
	"net/http"

	"github.com/swaggest/openapi-go"
)

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

type options struct {
	defaultStatusCode int
	openapi           []func(openapi.OperationContext)
}

// Option
type Option func(*options)

// Endpoint
type Endpoint[Req, Resp any] struct {
	method  string
	pattern string

	validators []func(*http.Request) error
	injectors  []func(context.Context, *http.Request) context.Context

	statusCode int
	handler    Handler[Req, Resp]

	openapi []func(openapi.OperationContext)
}

// ContentTyper
type ContentTyper interface {
	ContentType() string
}

// Accepts
func Accepts[Req any]() Option {
	return func(ho *options) {
		ho.openapi = append(ho.openapi, func(oc openapi.OperationContext) {
			contentType := ""

			var req Req
			if ct, ok := any(req).(ContentTyper); ok {
				contentType = ct.ContentType()
			}

			oc.AddReqStructure(req, func(cu *openapi.ContentUnit) {
				cu.ContentType = contentType
			})
		})
	}
}

var DefaultStatusCode = http.StatusOK

// StatusCode
func StatusCode(statusCode int) Option {
	return func(ho *options) {
		ho.defaultStatusCode = statusCode
	}
}

func returns(status int) func(oc openapi.OperationContext) {
	return func(oc openapi.OperationContext) {
		oc.AddRespStructure(
			nil,
			openapi.WithHTTPStatus(status),
		)
	}
}

// Returns
func Returns(status int) Option {
	return func(ho *options) {
		ho.openapi = append(ho.openapi, returns(status))
	}
}

func returnsWith[Resp any](resp Resp, contentType string, status int) func(oc openapi.OperationContext) {
	return func(oc openapi.OperationContext) {
		oc.AddRespStructure(
			resp,
			openapi.WithContentType(contentType),
			openapi.WithHTTPStatus(status),
		)
	}
}

// ReturnsWith
func ReturnsWith[Resp any](status int) Option {
	return func(ho *options) {
		contentType := ""

		var resp Resp
		if ct, ok := any(resp).(ContentTyper); ok {
			contentType = ct.ContentType()
		}

		ho.openapi = append(ho.openapi, returnsWith(resp, contentType, status))
	}
}

// New initializes an Endpoint.
func New[Req, Resp any](method string, pattern string, handler Handler[Req, Resp], opts ...Option) *Endpoint[Req, Resp] {
	o := &options{
		defaultStatusCode: DefaultStatusCode,
	}
	var req Req
	if _, ok := any(req).(ContentTyper); ok {
		opts = append(opts, Accepts[Req]())
	}

	var resp Resp
	if ct, ok := any(resp).(ContentTyper); ok {
		opts = append(opts, func(ho *options) {
			ho.openapi = append(ho.openapi, returnsWith(ct, ct.ContentType(), ho.defaultStatusCode))
		})
	} else {
		opts = append(opts, func(ho *options) {
			ho.openapi = append(ho.openapi, returns(ho.defaultStatusCode))
		})
	}

	for _, opt := range opts {
		opt(o)
	}

	return &Endpoint[Req, Resp]{
		method:     method,
		pattern:    pattern,
		statusCode: o.defaultStatusCode,
		handler:    handler,
		openapi:    o.openapi,
	}
}

func Get[Req, Resp any](pattern string, handler Handler[Req, Resp], opts ...Option) *Endpoint[Req, Resp] {
	return New(http.MethodGet, pattern, handler, opts...)
}

func Post[Req, Resp any](pattern string, handler Handler[Req, Resp], opts ...Option) *Endpoint[Req, Resp] {
	return New(http.MethodPost, pattern, handler, opts...)
}

func Put[Req, Resp any](pattern string, handler Handler[Req, Resp], opts ...Option) *Endpoint[Req, Resp] {
	return New(http.MethodPut, pattern, handler, opts...)
}

func Delete[Req, Resp any](pattern string, handler Handler[Req, Resp], opts ...Option) *Endpoint[Req, Resp] {
	return New(http.MethodDelete, pattern, handler, opts...)
}

func (e *Endpoint[Req, Resp]) Method() string {
	return e.method
}

func (e *Endpoint[Req, Resp]) Pattern() string {
	return e.pattern
}

func (e *Endpoint[Req, Resp]) OpenApi(oc openapi.OperationContext) {
	for _, opt := range e.openapi {
		opt(oc)
	}
}

// ServeHTTP implements the [http.Handler] interface.
func (e *Endpoint[Req, Resp]) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// [x] validate raw http request i.e. query params, headers, etc.
	// [x] unmarshal request body
	// [x] validate request body
	// [ ] propogate query params, path variables, and headers via context
	// [ ] custom response body and status code control for errors
	// [x] marshal response body

	err := validateRequest(r, e.validators...)
	if err != nil {
		// TODO
		return
	}

	var req Req
	err = unmarshal(r.Body, &req)
	if err != nil {
		// TODO
		return
	}

	err = validate(req)
	if err != nil {
		// TODO
		return
	}

	ctx := inject(r.Context(), r, e.injectors...)
	resp, err := e.handler.Handle(ctx, req)
	if err != nil {
		// TODO
		return
	}

	bm, ok := any(resp).(encoding.BinaryMarshaler)
	if !ok {
		return
	}

	b, err := bm.MarshalBinary()
	if err != nil {
		// TODO
		return
	}

	w.WriteHeader(e.statusCode)
	_, err = io.Copy(w, bytes.NewReader(b))
	if err != nil {
		// TODO
		return
	}
}

func validateRequest(r *http.Request, validators ...func(*http.Request) error) error {
	for _, validator := range validators {
		err := validator(r)
		if err != nil {
			return err
		}
	}
	return nil
}

func validateMethod(method string) func(*http.Request) error {
	return func(r *http.Request) error {
		if r.Method == method {
			return nil
		}
		return errors.New("invalid method")
	}
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

func inject(ctx context.Context, r *http.Request, injectors ...func(context.Context, *http.Request) context.Context) context.Context {
	for _, injector := range injectors {
		ctx = injector(ctx, r)
	}
	return ctx
}
