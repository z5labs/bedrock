// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package otelslog

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestHandler_Handle(t *testing.T) {
	t.Run("will not add trace id and span id", func(t *testing.T) {
		t.Run("if the span context is invalid", func(t *testing.T) {
			var buf bytes.Buffer
			log := New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{}))

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			log.InfoContext(ctx, "test")

			var record struct {
				Message string `json:"msg"`
				OTel    struct {
					TraceID string `json:"trace_id"`
					SpanID  string `json:"span_id"`
				} `json:"otel"`
			}
			err := json.Unmarshal(buf.Bytes(), &record)
			if !assert.Nil(t, err) {
				return
			}
			if !assert.Equal(t, "test", record.Message) {
				return
			}
			if !assert.Empty(t, record.OTel.TraceID) {
				return
			}
			if !assert.Empty(t, record.OTel.SpanID) {
				return
			}
		})
	})

	t.Run("will add trace id and span id", func(t *testing.T) {
		t.Run("if the span context is valid", func(t *testing.T) {
			var buf bytes.Buffer
			log := New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{}))

			exporter, err := stdouttrace.New(stdouttrace.WithWriter(io.Discard))
			if !assert.Nil(t, err) {
				return
			}
			tp := sdktrace.NewTracerProvider(
				sdktrace.WithBatcher(exporter),
				sdktrace.WithResource(resource.Default()),
			)
			otel.SetTracerProvider(tp)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			spanCtx, span := otel.Tracer("otelslog").Start(ctx, "test")
			if !assert.True(t, span.SpanContext().IsValid()) {
				return
			}

			log.InfoContext(spanCtx, "test")

			var record struct {
				Message string `json:"msg"`
				OTel    struct {
					TraceID string `json:"trace_id"`
					SpanID  string `json:"span_id"`
				} `json:"otel"`
			}
			err = json.Unmarshal(buf.Bytes(), &record)
			if !assert.Nil(t, err) {
				return
			}
			if !assert.Equal(t, "test", record.Message) {
				return
			}
			if !assert.Equal(t, span.SpanContext().TraceID().String(), record.OTel.TraceID) {
				t.Log(buf.String())
				return
			}
			if !assert.Equal(t, span.SpanContext().SpanID().String(), record.OTel.SpanID) {
				t.Log(buf.String())
				return
			}
		})
	})
}
