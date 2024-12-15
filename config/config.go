// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package config provides very easy to use and extensible configuration management capabilities.
package config

import (
	"encoding"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/z5labs/bedrock/config/key"

	"github.com/go-viper/mapstructure/v2"
)

// Store represents a general key value structure.
type Store interface {
	Set(key.Keyer, any) error
}

// Source defines valid config sources as those who can
// serialize themselves into a key value like structure.
type Source interface {
	Apply(Store) error
}

// Manager
type Manager struct {
	store Store
}

// Read
// Subsequent sources override previous sources.
func Read(srcs ...Source) (*Manager, error) {
	if len(srcs) == 0 {
		return &Manager{store: make(Map)}, nil
	}

	store := make(Map)
	for _, src := range srcs {
		err := src.Apply(store)
		if err != nil {
			return nil, err
		}
	}
	m := &Manager{
		store: store,
	}
	return m, nil
}

// Unmarshal
func (m *Manager) Unmarshal(v any) error {
	dec, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		TagName: "config",
		Result:  v,
		DecodeHook: composeDecodeHooks(
			textUnmarshalerHookFunc(),
			timeDurationHookFunc(),
		),
	})
	if err != nil {
		return err
	}
	return dec.Decode(m.store)
}

var errInvalidDecodeCondition = errors.New("invalid decode condition")

// TypeCoercionError occurs when attempting to unmarshal a config
// value to a struct field whose type does not match the config
// value type, up to, coercion.
type TypeCoercionError struct {
	from  reflect.Value
	to    reflect.Value
	Cause error
}

// Error implements the error interface.
func (e TypeCoercionError) Error() string {
	return fmt.Sprintf("failed to coerce value from %s to %s: %s", e.from.Type().Name(), e.to.Type().Name(), e.Cause)
}

// Unwrap implements the implicit interface for usage with errors.Is and errors.As.
func (e TypeCoercionError) Unwrap() error {
	return e.Cause
}

func composeDecodeHooks(hs ...mapstructure.DecodeHookFunc) mapstructure.DecodeHookFuncValue {
	return func(f, t reflect.Value) (any, error) {
		for _, h := range hs {
			v, err := mapstructure.DecodeHookExec(h, f, t)
			if err == nil {
				return v, nil
			}
			if err == errInvalidDecodeCondition {
				continue
			}
			return nil, TypeCoercionError{
				from:  f,
				to:    t,
				Cause: err,
			}
		}
		return f.Interface(), nil
	}
}

func textUnmarshalerHookFunc() mapstructure.DecodeHookFuncType {
	return func(f reflect.Type, t reflect.Type, data any) (any, error) {
		if f.Kind() != reflect.String {
			return nil, errInvalidDecodeCondition
		}
		result := reflect.New(t).Interface()
		u, ok := result.(encoding.TextUnmarshaler)
		if !ok {
			return nil, errInvalidDecodeCondition
		}
		err := u.UnmarshalText([]byte(data.(string)))
		if err != nil {
			return nil, err
		}
		return result, nil
	}
}

func timeDurationHookFunc() mapstructure.DecodeHookFuncType {
	return func(f reflect.Type, t reflect.Type, data any) (any, error) {
		if t != reflect.TypeOf(time.Duration(0)) {
			return nil, errInvalidDecodeCondition
		}

		switch f.Kind() {
		case reflect.String:
			return time.ParseDuration(data.(string))
		case reflect.Int:
			return time.Duration(int64(data.(int))), nil
		default:
			return nil, errInvalidDecodeCondition
		}
	}
}
