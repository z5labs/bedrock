// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package sqsslog

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMessageId(t *testing.T) {
	t.Run("will marshal as a string", func(t *testing.T) {
		t.Run("if marshalling to json", func(t *testing.T) {
			var buf bytes.Buffer
			log := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{}))

			log.Info("hello", MessageId("1234"))

			var record struct {
				MessageId string `json:"sqs_message_id"`
			}
			err := json.Unmarshal(buf.Bytes(), &record)
			if !assert.Nil(t, err) {
				return
			}
			if !assert.Equal(t, "1234", record.MessageId) {
				return
			}
		})
	})
}

func TestMessageIds(t *testing.T) {
	t.Run("will marshal as a list of strings", func(t *testing.T) {
		t.Run("if marshalling to json", func(t *testing.T) {
			var buf bytes.Buffer
			log := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{}))

			ids := []string{"1234", "5678"}
			log.Info("hello", MessageIds(ids))

			var record struct {
				MessageIds []string `json:"sqs_message_ids"`
			}
			err := json.Unmarshal(buf.Bytes(), &record)
			if !assert.Nil(t, err) {
				return
			}
			if !assert.Len(t, record.MessageIds, len(ids)) {
				return
			}
			if !assert.Equal(t, ids, record.MessageIds) {
				return
			}
		})
	})
}

func TestReceiptHandle(t *testing.T) {
	t.Run("will marshal as a string", func(t *testing.T) {
		t.Run("if marshalling to json", func(t *testing.T) {
			var buf bytes.Buffer
			log := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{}))

			log.Info("hello", ReceiptHandle("1234"))

			var record struct {
				ReceiptHandle string `json:"sqs_receipt_handle"`
			}
			err := json.Unmarshal(buf.Bytes(), &record)
			if !assert.Nil(t, err) {
				return
			}
			if !assert.Equal(t, "1234", record.ReceiptHandle) {
				return
			}
		})
	})
}

func TestMessageAttributes(t *testing.T) {
	t.Run("will marshal as object", func(t *testing.T) {
		t.Run("if marshalling to json", func(t *testing.T) {
			var buf bytes.Buffer
			log := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{}))

			attrs := map[string]string{"1234": "5678"}
			log.Info("hello", MessageAttributes(attrs))

			var record struct {
				MessageAttributes map[string]string `json:"sqs_message_attributes"`
			}
			err := json.Unmarshal(buf.Bytes(), &record)
			if !assert.Nil(t, err) {
				return
			}
			if !assert.Len(t, record.MessageAttributes, len(attrs)) {
				return
			}
			if !assert.Equal(t, attrs, record.MessageAttributes) {
				return
			}
		})
	})
}
