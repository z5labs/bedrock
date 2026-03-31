// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package rest

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/swaggest/openapi-go/openapi3"
)

// paramKey is a unique identity for a parameter in the paramStore.
// The unexported field ensures each &paramKey[T]{} allocation gets a distinct address,
// since Go may reuse addresses for zero-size types.
type paramKey[T any] struct{ _ int }

// Param represents a declared API parameter (path, query, or header).
// It serves dual purpose: composing into the endpoint chain via Read,
// and retrieving the decoded value in the handler via ParamFrom.
type Param[T any] struct {
	name string
	in   string // "path", "query", "header"
	opts paramOptions
	key  *paramKey[T]
}

type paramOptions struct {
	required      *bool
	description   string
	example       any
	pattern       string
	patternRegexp *regexp.Regexp
	minimum       *float64
	maximum       *float64
	minLength     *int
	maxLength     *int
	enumValues    []any
	defaultVal    any
}

// ParamOption configures a parameter's metadata and validation constraints.
type ParamOption func(*paramOptions)

// Required marks a parameter as required.
func Required() ParamOption {
	return func(o *paramOptions) {
		t := true
		o.required = &t
	}
}

// Optional marks a parameter as optional.
func Optional() ParamOption {
	return func(o *paramOptions) {
		f := false
		o.required = &f
	}
}

// ParamDescription sets the parameter description in the OpenAPI spec.
func ParamDescription(desc string) ParamOption {
	return func(o *paramOptions) {
		o.description = desc
	}
}

// ParamExample sets an example value for the parameter in the OpenAPI spec.
func ParamExample(ex any) ParamOption {
	return func(o *paramOptions) {
		o.example = ex
	}
}

// Pattern sets a regex pattern constraint on the parameter value.
// The pattern is precompiled at declaration time for efficiency.
func Pattern(regex string) ParamOption {
	return func(o *paramOptions) {
		o.pattern = regex
		// Precompile the regex at declaration time.
		// If invalid, it will be caught during param creation.
		re, err := regexp.Compile(regex)
		if err == nil {
			o.patternRegexp = re
		}
	}
}

// Minimum sets the minimum value constraint for numeric parameters.
func Minimum(n float64) ParamOption {
	return func(o *paramOptions) {
		o.minimum = &n
	}
}

// Maximum sets the maximum value constraint for numeric parameters.
func Maximum(n float64) ParamOption {
	return func(o *paramOptions) {
		o.maximum = &n
	}
}

// MinLength sets the minimum length constraint for string parameters.
func MinLength(n int) ParamOption {
	return func(o *paramOptions) {
		o.minLength = &n
	}
}

// MaxLength sets the maximum length constraint for string parameters.
func MaxLength(n int) ParamOption {
	return func(o *paramOptions) {
		o.maxLength = &n
	}
}

// Enum constrains the parameter to a set of allowed values.
func Enum[T any](values ...T) ParamOption {
	return func(o *paramOptions) {
		o.enumValues = make([]any, len(values))
		for i, v := range values {
			o.enumValues[i] = v
		}
	}
}

// DefaultValue sets the default value for the parameter when not provided.
func DefaultValue[T any](val T) ParamOption {
	return func(o *paramOptions) {
		o.defaultVal = val
	}
}

// PathParam declares a path parameter with the given name and options.
// Path parameters are required by default.
func PathParam[T any](name string, opts ...ParamOption) Param[T] {
	o := paramOptions{}
	t := true
	o.required = &t // path params required by default
	for _, opt := range opts {
		opt(&o)
	}
	return Param[T]{
		name: name,
		in:   "path",
		opts: o,
		key:  &paramKey[T]{},
	}
}

// QueryParam declares a query parameter with the given name and options.
// Query parameters are optional by default.
func QueryParam[T any](name string, opts ...ParamOption) Param[T] {
	o := paramOptions{}
	f := false
	o.required = &f // query params optional by default
	for _, opt := range opts {
		opt(&o)
	}
	return Param[T]{
		name: name,
		in:   "query",
		opts: o,
		key:  &paramKey[T]{},
	}
}

// HeaderParam declares a header parameter with the given name and options.
// Header parameters are optional by default.
func HeaderParam[T any](name string, opts ...ParamOption) Param[T] {
	o := paramOptions{}
	f := false
	o.required = &f // header params optional by default
	for _, opt := range opts {
		opt(&o)
	}
	return Param[T]{
		name: name,
		in:   "header",
		opts: o,
		key:  &paramKey[T]{},
	}
}

// Read composes this parameter into the endpoint chain.
// It registers the parameter in the OpenAPI spec and adds runtime decoding + validation.
func (p Param[T]) Read(ep Endpoint) Endpoint {
	ep.readers = append(ep.readers, p.makeReader())
	ep.specOps = append(ep.specOps, p.makeSpecOp())
	return ep
}

func (p Param[T]) makeReader() func(*http.Request, *paramStore) error {
	return func(r *http.Request, store *paramStore) error {
		raw, found := p.extractRaw(r)
		if !found || raw == "" {
			if p.opts.defaultVal != nil {
				store.set(p.key, p.opts.defaultVal)
				return nil
			}
			if p.opts.required != nil && *p.opts.required {
				return &ValidationError{
					Param:   p.name,
					Message: fmt.Sprintf("required %s parameter %q is missing", p.in, p.name),
				}
			}
			var zero T
			store.set(p.key, zero)
			return nil
		}

		val, err := parseValue[T](raw)
		if err != nil {
			return &ValidationError{
				Param:   p.name,
				Message: fmt.Sprintf("invalid value for %s parameter %q: %v", p.in, p.name, err),
			}
		}

		if err := p.validate(raw, val); err != nil {
			return err
		}

		store.set(p.key, val)
		return nil
	}
}

func (p Param[T]) extractRaw(r *http.Request) (string, bool) {
	switch p.in {
	case "path":
		v := chi.URLParam(r, p.name)
		return v, v != ""
	case "query":
		if !r.URL.Query().Has(p.name) {
			return "", false
		}
		return r.URL.Query().Get(p.name), true
	case "header":
		v := r.Header.Get(p.name)
		return v, v != ""
	default:
		return "", false
	}
}

func (p Param[T]) validate(raw string, val any) error {
	if p.opts.pattern != "" {
		re := p.opts.patternRegexp
		if re == nil {
			// Pattern was invalid at declaration time.
			return &ValidationError{
				Param:   p.name,
				Message: fmt.Sprintf("invalid pattern for parameter %q: pattern failed to compile", p.name),
			}
		}
		if !re.MatchString(raw) {
			return &ValidationError{
				Param:   p.name,
				Message: fmt.Sprintf("parameter %q value %q does not match pattern %q", p.name, raw, p.opts.pattern),
			}
		}
	}

	if p.opts.minLength != nil {
		if len(raw) < *p.opts.minLength {
			return &ValidationError{
				Param:   p.name,
				Message: fmt.Sprintf("parameter %q value is shorter than minimum length %d", p.name, *p.opts.minLength),
			}
		}
	}

	if p.opts.maxLength != nil {
		if len(raw) > *p.opts.maxLength {
			return &ValidationError{
				Param:   p.name,
				Message: fmt.Sprintf("parameter %q value exceeds maximum length %d", p.name, *p.opts.maxLength),
			}
		}
	}

	if p.opts.minimum != nil || p.opts.maximum != nil {
		numVal, err := toFloat64(val)
		if err == nil {
			if p.opts.minimum != nil && numVal < *p.opts.minimum {
				return &ValidationError{
					Param:   p.name,
					Message: fmt.Sprintf("parameter %q value %v is less than minimum %v", p.name, numVal, *p.opts.minimum),
				}
			}
			if p.opts.maximum != nil && numVal > *p.opts.maximum {
				return &ValidationError{
					Param:   p.name,
					Message: fmt.Sprintf("parameter %q value %v exceeds maximum %v", p.name, numVal, *p.opts.maximum),
				}
			}
		}
	}

	if len(p.opts.enumValues) > 0 {
		found := false
		for _, allowed := range p.opts.enumValues {
			if fmt.Sprintf("%v", allowed) == raw {
				found = true
				break
			}
		}
		if !found {
			return &ValidationError{
				Param:   p.name,
				Message: fmt.Sprintf("parameter %q value %q is not one of the allowed values", p.name, raw),
			}
		}
	}

	return nil
}

func (p Param[T]) makeSpecOp() func(*openapi3.Operation) {
	return func(op *openapi3.Operation) {
		schema := schemaForType[T]()
		applyConstraints(schema, p.opts)

		param := openapi3.Parameter{
			Name:   p.name,
			In:     openapi3.ParameterIn(p.in),
			Schema: &openapi3.SchemaOrRef{Schema: schema},
		}

		if p.opts.required != nil {
			param.WithRequired(*p.opts.required)
		}
		if p.opts.description != "" {
			param.WithDescription(p.opts.description)
		}
		if p.opts.example != nil {
			param.Schema.Schema.WithExample(p.opts.example)
		}
		if p.opts.defaultVal != nil {
			param.Schema.Schema.WithDefault(p.opts.defaultVal)
		}

		op.Parameters = append(op.Parameters, openapi3.ParameterOrRef{Parameter: &param})
	}
}

// ValidationError is returned when parameter validation fails.
type ValidationError struct {
	Param   string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}

// parseValue converts a raw string to type T.
func parseValue[T any](raw string) (T, error) {
	var zero T
	v := any(&zero)

	switch target := v.(type) {
	case *string:
		*target = raw
		return zero, nil
	case *int:
		n, err := strconv.Atoi(raw)
		if err != nil {
			return zero, err
		}
		*target = n
		return zero, nil
	case *int64:
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return zero, err
		}
		*target = n
		return zero, nil
	case *float64:
		n, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return zero, err
		}
		*target = n
		return zero, nil
	case *bool:
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return zero, err
		}
		*target = b
		return zero, nil
	default:
		return zero, fmt.Errorf("unsupported parameter type %T", zero)
	}
}

func toFloat64(v any) (float64, error) {
	switch n := v.(type) {
	case int:
		return float64(n), nil
	case int64:
		return float64(n), nil
	case float64:
		return n, nil
	default:
		return 0, fmt.Errorf("cannot convert %T to float64", v)
	}
}

// schemaForType returns an OpenAPI schema for the given Go type.
func schemaForType[T any]() *openapi3.Schema {
	var zero T
	switch any(zero).(type) {
	case string:
		return &openapi3.Schema{Type: ptrSchemaType(openapi3.SchemaTypeString)}
	case int, int64:
		return &openapi3.Schema{Type: ptrSchemaType(openapi3.SchemaTypeInteger)}
	case float64:
		return &openapi3.Schema{Type: ptrSchemaType(openapi3.SchemaTypeNumber)}
	case bool:
		return &openapi3.Schema{Type: ptrSchemaType(openapi3.SchemaTypeBoolean)}
	default:
		return &openapi3.Schema{Type: ptrSchemaType(openapi3.SchemaTypeString)}
	}
}

func ptrSchemaType(t openapi3.SchemaType) *openapi3.SchemaType {
	return &t
}

func applyConstraints(schema *openapi3.Schema, opts paramOptions) {
	if opts.pattern != "" {
		schema.WithPattern(opts.pattern)
	}
	if opts.minimum != nil {
		schema.WithMinimum(*opts.minimum)
	}
	if opts.maximum != nil {
		schema.WithMaximum(*opts.maximum)
	}
	if opts.minLength != nil {
		schema.WithMinLength(int64(*opts.minLength))
	}
	if opts.maxLength != nil {
		schema.WithMaxLength(int64(*opts.maxLength))
	}
	if len(opts.enumValues) > 0 {
		schema.WithEnum(opts.enumValues...)
	}
}
