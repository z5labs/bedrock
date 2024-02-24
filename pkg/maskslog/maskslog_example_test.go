// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package maskslog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
)

func ExampleHandler_Handle_message() {
	var buf bytes.Buffer

	h := NewHandler(
		slog.NewJSONHandler(&buf, &slog.HandlerOptions{}),
		Message(func(s string) string {
			ss := strings.Split(s, " ")
			return strings.Join(append(ss[0:1], "****"), " ")
		}),
	)
	logger := slog.New(h)

	logger.Info("hello world!")

	var record struct {
		Message string `json:"msg"`
	}
	err := json.Unmarshal(buf.Bytes(), &record)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(record.Message)
	// Output: hello ****
}

func ExampleHandler_Handle_attr() {
	var buf bytes.Buffer

	h := NewHandler(
		slog.NewJSONHandler(&buf, &slog.HandlerOptions{}),
		Attr("secret", AnonymousStringAttr),
	)
	logger := slog.New(h)

	logger.Info("hello world!", slog.String("secret", "super duper secret value"))

	var record struct {
		Secret string `json:"secret"`
	}
	err := json.Unmarshal(buf.Bytes(), &record)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(record.Secret)
	// Output: ****
}
