// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package rest

import (
	"bytes"
	"context"
	"encoding"
	"errors"
	"io"
	"net/http"

	"github.com/swaggest/openapi-go"
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

// ContentTyper
type ContentTyper interface {
	ContentType() string
}

type handleOptions struct {
	statusCode int
	ocOpts     []func(openapi.OperationContext)
	validators []func(*http.Request) error
	injectors  []func(context.Context, *http.Request) context.Context
}

// HandleOption
type HandleOption func(*handleOptions)

// Accepts
func Accepts[Req any]() HandleOption {
	return func(ho *handleOptions) {
		ho.ocOpts = append(ho.ocOpts, func(oc openapi.OperationContext) {
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

// DefaultStatusCode
func DefaultStatusCode(statusCode int) HandleOption {
	return func(ho *handleOptions) {
		ho.statusCode = statusCode
	}
}

// Returns
func Returns(status int) HandleOption {
	return func(ho *handleOptions) {
		ho.ocOpts = append(ho.ocOpts, func(oc openapi.OperationContext) {
			oc.AddRespStructure(nil, func(cu *openapi.ContentUnit) {
				cu.HTTPStatus = status
			})
		})
	}
}

// ReturnsWith
func ReturnsWith[Resp any](status int) HandleOption {
	return func(ho *handleOptions) {
		ho.ocOpts = append(ho.ocOpts, func(oc openapi.OperationContext) {
			contentType := ""

			var resp Resp
			if ct, ok := any(resp).(ContentTyper); ok {
				contentType = ct.ContentType()
			}

			oc.AddRespStructure(resp, func(cu *openapi.ContentUnit) {
				cu.ContentType = contentType
				cu.HTTPStatus = status
			})
		})
	}
}

// Handle
func Handle[Req, Resp any](method string, pattern string, handler Handler[Req, Resp], opts ...HandleOption) Option {
	// [x] openapi method
	// [x] openapi pattern
	// [ ] openapi headers
	// [ ] openapi query params
	// [ ] openapi auth
	// [x] openapi request body
	// [x] openapi response body and content type
	// [x] openapi response status code
	// [x] lift and register handler with http.ServeMux

	ho := &handleOptions{
		statusCode: http.StatusOK,
	}
	ho.validators = append(ho.validators, validateMethod(method))

	var req Req
	if _, ok := any(&req).(encoding.BinaryUnmarshaler); ok {
		opts = append(opts, Accepts[Req]())
	}

	var resp Resp
	if _, ok := any(resp).(encoding.BinaryMarshaler); ok {
		opts = append(opts, ReturnsWith[Resp](ho.statusCode))
	}

	for _, opt := range opts {
		opt(ho)
	}

	return func(app *App) {
		oc, err := app.openapi.NewOperationContext(method, pattern)
		if err != nil {
			panic(err)
		}
		for _, opt := range ho.ocOpts {
			opt(oc)
		}
		app.openapi.AddOperation(oc)

		app.mux.Handle(pattern, liftHandler(ho, handler))
	}
}

// Validator
type Validator interface {
	Validate() error
}

func liftHandler[Req, Resp any](ho *handleOptions, h Handler[Req, Resp]) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// [x] validate raw http request i.e. query params, headers, etc.
		// [x] unmarshal request body
		// [x] validate request body
		// [ ] propogate query params, path variables, and headers via context
		// [ ] custom response body and status code control for errors
		// [x] marshal response body

		err := validateRequest(r, ho.validators...)
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

		ctx := inject(r.Context(), r, ho.injectors...)
		resp, err := h.Handle(ctx, req)
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

		w.WriteHeader(ho.statusCode)
		_, err = io.Copy(w, bytes.NewReader(b))
		if err != nil {
			// TODO
			return
		}
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
