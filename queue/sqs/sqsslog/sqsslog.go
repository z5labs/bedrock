// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package sqsslog

import (
	"log/slog"

	"github.com/z5labs/bedrock/pkg/slogfield"
)

// MessageId returns a slog.Attr for the SQS message id.
func MessageId(s string) slog.Attr {
	return slog.String("sqs_message_id", s)
}

// MessageIds returns a slog.Attr for multiple SQS message ids.
func MessageIds(ss []string) slog.Attr {
	return slogfield.Strings("sqs_message_ids", ss)
}

// ReceiptHandle returns a slog.Attr for the SQS message receipt handle
func ReceiptHandle(s string) slog.Attr {
	return slog.String("sqs_receipt_handle", s)
}

// MessageAttributes returns a slog.Attr for the SQS message attributes.
func MessageAttributes(m map[string]string) slog.Attr {
	attrs := make([]any, len(m))
	for key, val := range m {
		attrs = append(attrs, slog.String(key, val))
	}
	return slog.Group("sqs_message_attributes", attrs...)
}
