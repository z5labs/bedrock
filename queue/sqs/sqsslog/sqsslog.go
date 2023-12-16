// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package sqsslog

import "log/slog"

// MessageId
func MessageId(s string) slog.Attr {
	return slog.String("sqs_message_id", s)
}

// ReceiptHandle
func ReceiptHandle(s string) slog.Attr {
	return slog.String("sqs_receipt_handle", s)
}

// MessageAttributes
func MessageAttributes(m map[string]string) slog.Attr {
	attrs := make([]any, len(m))
	for key, val := range m {
		attrs = append(attrs, slog.String(key, val))
	}
	return slog.Group("sqs_message_attributes", attrs...)
}
