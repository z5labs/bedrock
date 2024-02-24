// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package maskslog

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHandler_Handle(t *testing.T) {
	t.Run("will not mask attrs", func(t *testing.T) {
		t.Run("if no masking funcs are registered", func(t *testing.T) {
			var buf bytes.Buffer
			h := NewHandler(slog.NewJSONHandler(&buf, &slog.HandlerOptions{}))

			logger := slog.New(h)
			logger.Info("hello world", slog.String("secret", "super duper secret value"))

			var record struct {
				Message string `json:"msg"`
				Secret  string `json:"secret"`
			}
			err := json.Unmarshal(buf.Bytes(), &record)
			if !assert.Nil(t, err) {
				return
			}
			if !assert.Equal(t, "hello world", record.Message) {
				return
			}
			if !assert.Equal(t, "super duper secret value", record.Secret) {
				return
			}
		})

		t.Run("if slog.Attr key does not match a masking func", func(t *testing.T) {
			var buf bytes.Buffer
			h := NewHandler(
				slog.NewJSONHandler(&buf, &slog.HandlerOptions{}),
				Attr("random", AnonymousStringAttr),
			)

			logger := slog.New(h)
			logger.Info("hello world", slog.String("secret", "super duper secret value"))

			var record struct {
				Message string `json:"msg"`
				Secret  string `json:"secret"`
			}
			err := json.Unmarshal(buf.Bytes(), &record)
			if !assert.Nil(t, err) {
				return
			}
			if !assert.Equal(t, "hello world", record.Message) {
				return
			}
			if !assert.Equal(t, "super duper secret value", record.Secret) {
				return
			}
		})
	})

	t.Run("will mask attrs", func(t *testing.T) {})
}

func TestHandler_WithAttrs(t *testing.T) {
	t.Run("will not mask attrs", func(t *testing.T) {
		t.Run("if there are no masking funcs", func(t *testing.T) {
			var buf bytes.Buffer
			var h slog.Handler = NewHandler(
				slog.NewJSONHandler(&buf, &slog.HandlerOptions{}),
			)
			h = h.WithAttrs([]slog.Attr{slog.String("secret", "super duper secret value")})

			logger := slog.New(h)
			logger.Info("hello world")

			var record struct {
				Message string `json:"msg"`
				Secret  string `json:"secret"`
			}
			err := json.Unmarshal(buf.Bytes(), &record)
			if !assert.Nil(t, err) {
				return
			}
			if !assert.Equal(t, "hello world", record.Message) {
				return
			}
			if !assert.Equal(t, "super duper secret value", record.Secret) {
				return
			}
		})

		t.Run("if none of the keys match a registered masking func", func(t *testing.T) {
			var buf bytes.Buffer
			var h slog.Handler = NewHandler(
				slog.NewJSONHandler(&buf, &slog.HandlerOptions{}),
				Attr("random", AnonymousStringAttr),
			)
			h = h.WithAttrs([]slog.Attr{slog.String("secret", "super duper secret value")})

			logger := slog.New(h)
			logger.Info("hello world")

			var record struct {
				Message string `json:"msg"`
				Secret  string `json:"secret"`
			}
			err := json.Unmarshal(buf.Bytes(), &record)
			if !assert.Nil(t, err) {
				return
			}
			if !assert.Equal(t, "hello world", record.Message) {
				return
			}
			if !assert.Equal(t, "super duper secret value", record.Secret) {
				return
			}
		})
	})

	t.Run("will mask attrs", func(t *testing.T) {
		t.Run("if any of the keys match a registered masking func", func(t *testing.T) {
			var buf bytes.Buffer
			var h slog.Handler = NewHandler(
				slog.NewJSONHandler(&buf, &slog.HandlerOptions{}),
				Attr("secret", AnonymousStringAttr),
			)
			h = h.WithAttrs([]slog.Attr{slog.String("secret", "super duper secret value")})

			logger := slog.New(h)
			logger.Info("hello world")

			var record struct {
				Message string `json:"msg"`
				Secret  string `json:"secret"`
			}
			err := json.Unmarshal(buf.Bytes(), &record)
			if !assert.Nil(t, err) {
				return
			}
			if !assert.Equal(t, "hello world", record.Message) {
				return
			}
			if !assert.Equal(t, "****", record.Secret) {
				return
			}
		})
	})
}

func TestHandler_WithGroup(t *testing.T) {
	t.Run("will not mask entire group", func(t *testing.T) {
		t.Run("if the group name matchs a registered masking func", func(t *testing.T) {
			var buf bytes.Buffer
			var h slog.Handler = NewHandler(
				slog.NewJSONHandler(&buf, &slog.HandlerOptions{}),
				Attr("secret", AnonymousStringAttr),
			)
			h = h.WithGroup("secret")

			logger := slog.New(h)
			logger.Info("hello world", slog.String("a", "value"))

			var record struct {
				Message string `json:"msg"`
				Secret  struct {
					A string `json:"a"`
				} `json:"secret"`
			}
			err := json.Unmarshal(buf.Bytes(), &record)
			if !assert.Nil(t, err) {
				t.Log(buf.String())
				return
			}
			if !assert.Equal(t, "hello world", record.Message) {
				return
			}
			if !assert.Equal(t, "value", record.Secret.A) {
				return
			}
		})
	})

	t.Run("will mask sub-attrs", func(t *testing.T) {
		t.Run("if any of the keys match a registered masking func", func(t *testing.T) {
			var buf bytes.Buffer
			var h slog.Handler = NewHandler(
				slog.NewJSONHandler(&buf, &slog.HandlerOptions{}),
				Attr("a", AnonymousStringAttr),
			)
			h = h.WithGroup("secret")

			logger := slog.New(h)
			logger.Info("hello world", slog.String("a", "super duper secret value"))

			var record struct {
				Message string `json:"msg"`
				Secret  struct {
					A string `json:"a"`
				} `json:"secret"`
			}
			err := json.Unmarshal(buf.Bytes(), &record)
			if !assert.Nil(t, err) {
				return
			}
			if !assert.Equal(t, "hello world", record.Message) {
				return
			}
			if !assert.Equal(t, "****", record.Secret.A) {
				return
			}
		})
	})
}
