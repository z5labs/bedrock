// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package slogfield

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type logFields[T any] struct {
	Value  T   `json:"value"`
	Values []T `json:"values"`
}

func TestJsonHandler(t *testing.T) {
	testCases := []struct {
		Name     string
		Attrs    []any
		Validate func(*testing.T, *bytes.Buffer)
	}{
		{
			Name: "any",
			Attrs: []any{
				Any("value", true),
			},
			Validate: func(t *testing.T, buf *bytes.Buffer) {
				var res logFields[any]
				err := json.Unmarshal(buf.Bytes(), &res)
				if !assert.Nil(t, err) {
					return
				}
				if !assert.Equal(t, true, res.Value) {
					return
				}
			},
		},
		{
			Name: "duration",
			Attrs: []any{
				Duration("value", 5*time.Second),
			},
			Validate: func(t *testing.T, buf *bytes.Buffer) {
				var res logFields[time.Duration]
				err := json.Unmarshal(buf.Bytes(), &res)
				if !assert.Nil(t, err) {
					return
				}
				if !assert.Equal(t, 5*time.Second, res.Value) {
					return
				}
			},
		},
		{
			Name: "error",
			Attrs: []any{
				Error(errors.New("hello, world")),
			},
			Validate: func(t *testing.T, buf *bytes.Buffer) {
				var res struct {
					Err string `json:"error"`
				}
				err := json.Unmarshal(buf.Bytes(), &res)
				if !assert.Nil(t, err) {
					return
				}
				if !assert.Equal(t, "hello, world", res.Err) {
					return
				}
			},
		},
		{
			Name: "bool and bools",
			Attrs: []any{
				Bool("value", true),
				Bools("values", []bool{true, false}),
			},
			Validate: func(t *testing.T, buf *bytes.Buffer) {
				var res logFields[bool]
				err := json.Unmarshal(buf.Bytes(), &res)
				if !assert.Nil(t, err) {
					return
				}
				if !assert.Equal(t, true, res.Value) {
					return
				}
				if !assert.Equal(t, []bool{true, false}, res.Values) {
					return
				}
			},
		},
		{
			Name: "string and strings",
			Attrs: []any{
				String("value", "world"),
				Strings("values", []string{"a", "b"}),
			},
			Validate: func(t *testing.T, buf *bytes.Buffer) {
				var res logFields[string]
				err := json.Unmarshal(buf.Bytes(), &res)
				if !assert.Nil(t, err) {
					return
				}
				if !assert.Equal(t, "world", res.Value) {
					return
				}
				if !assert.Equal(t, []string{"a", "b"}, res.Values) {
					return
				}
			},
		},
		{
			Name: "int and ints",
			Attrs: []any{
				Int("value", 1),
				Ints("values", []int{0, 1}),
			},
			Validate: func(t *testing.T, buf *bytes.Buffer) {
				var res logFields[int]
				err := json.Unmarshal(buf.Bytes(), &res)
				if !assert.Nil(t, err) {
					return
				}
				if !assert.Equal(t, 1, res.Value) {
					return
				}
				if !assert.Equal(t, []int{0, 1}, res.Values) {
					return
				}
			},
		},
		{
			Name: "int8 and int8s",
			Attrs: []any{
				Int8("value", 1),
				Int8s("values", []int8{0, 1}),
			},
			Validate: func(t *testing.T, buf *bytes.Buffer) {
				var res logFields[int8]
				err := json.Unmarshal(buf.Bytes(), &res)
				if !assert.Nil(t, err) {
					return
				}
				if !assert.Equal(t, int8(1), res.Value) {
					return
				}
				if !assert.Equal(t, []int8{0, 1}, res.Values) {
					return
				}
			},
		},
		{
			Name: "int16 and int16s",
			Attrs: []any{
				Int16("value", 1),
				Int16s("values", []int16{0, 1}),
			},
			Validate: func(t *testing.T, buf *bytes.Buffer) {
				var res logFields[int16]
				err := json.Unmarshal(buf.Bytes(), &res)
				if !assert.Nil(t, err) {
					return
				}
				if !assert.Equal(t, int16(1), res.Value) {
					return
				}
				if !assert.Equal(t, []int16{0, 1}, res.Values) {
					return
				}
			},
		},
		{
			Name: "int32 and int32s",
			Attrs: []any{
				Int32("value", 1),
				Int32s("values", []int32{0, 1}),
			},
			Validate: func(t *testing.T, buf *bytes.Buffer) {
				var res logFields[int32]
				err := json.Unmarshal(buf.Bytes(), &res)
				if !assert.Nil(t, err) {
					return
				}
				if !assert.Equal(t, int32(1), res.Value) {
					return
				}
				if !assert.Equal(t, []int32{0, 1}, res.Values) {
					return
				}
			},
		},
		{
			Name: "int64 and int64s",
			Attrs: []any{
				Int64("value", 1),
				Int64s("values", []int64{0, 1}),
			},
			Validate: func(t *testing.T, buf *bytes.Buffer) {
				var res logFields[int64]
				err := json.Unmarshal(buf.Bytes(), &res)
				if !assert.Nil(t, err) {
					return
				}
				if !assert.Equal(t, int64(1), res.Value) {
					return
				}
				if !assert.Equal(t, []int64{0, 1}, res.Values) {
					return
				}
			},
		},
		{
			Name: "uint and uints",
			Attrs: []any{
				Uint("value", 1),
				Uints("values", []uint{0, 1}),
			},
			Validate: func(t *testing.T, buf *bytes.Buffer) {
				var res logFields[uint]
				err := json.Unmarshal(buf.Bytes(), &res)
				if !assert.Nil(t, err) {
					return
				}
				if !assert.Equal(t, uint(1), res.Value) {
					return
				}
				if !assert.Equal(t, []uint{0, 1}, res.Values) {
					return
				}
			},
		},
		{
			Name: "uint8 and uint8s",
			Attrs: []any{
				Uint8("value", 1),
				Uint8s("values", []uint8{0, 1}),
			},
			Validate: func(t *testing.T, buf *bytes.Buffer) {
				var res logFields[uint8]
				err := json.Unmarshal(buf.Bytes(), &res)
				if !assert.Nil(t, err) {
					return
				}
				if !assert.Equal(t, uint8(1), res.Value) {
					return
				}
				if !assert.Equal(t, []uint8{0, 1}, res.Values) {
					return
				}
			},
		},
		{
			Name: "uint16 and uint16s",
			Attrs: []any{
				Uint16("value", 1),
				Uint16s("values", []uint16{0, 1}),
			},
			Validate: func(t *testing.T, buf *bytes.Buffer) {
				var res logFields[uint16]
				err := json.Unmarshal(buf.Bytes(), &res)
				if !assert.Nil(t, err) {
					return
				}
				if !assert.Equal(t, uint16(1), res.Value) {
					return
				}
				if !assert.Equal(t, []uint16{0, 1}, res.Values) {
					return
				}
			},
		},
		{
			Name: "uint32 and uint32s",
			Attrs: []any{
				Uint32("value", 1),
				Uint32s("values", []uint32{0, 1}),
			},
			Validate: func(t *testing.T, buf *bytes.Buffer) {
				var res logFields[uint32]
				err := json.Unmarshal(buf.Bytes(), &res)
				if !assert.Nil(t, err) {
					return
				}
				if !assert.Equal(t, uint32(1), res.Value) {
					return
				}
				if !assert.Equal(t, []uint32{0, 1}, res.Values) {
					return
				}
			},
		},
		{
			Name: "uint64 and uint64s",
			Attrs: []any{
				Uint64("value", 1),
				Uint64s("values", []uint64{0, 1}),
			},
			Validate: func(t *testing.T, buf *bytes.Buffer) {
				var res logFields[uint64]
				err := json.Unmarshal(buf.Bytes(), &res)
				if !assert.Nil(t, err) {
					return
				}
				if !assert.Equal(t, uint64(1), res.Value) {
					return
				}
				if !assert.Equal(t, []uint64{0, 1}, res.Values) {
					return
				}
			},
		},
	}

	for _, testCase := range testCases {
		testCase := testCase

		t.Run(testCase.Name, func(t *testing.T) {
			var buf bytes.Buffer
			h := slog.NewJSONHandler(&buf, &slog.HandlerOptions{})
			logger := slog.New(h)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			logger.InfoContext(ctx, "test", testCase.Attrs...)

			testCase.Validate(t, &buf)
		})
	}
}
