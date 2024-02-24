// Copyright (c) 2024 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package otelslog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
)

func ExampleHandler_WithAttrs() {
	var buf bytes.Buffer
	var h slog.Handler = NewHandler(slog.NewJSONHandler(&buf, &slog.HandlerOptions{}))
	h = h.WithAttrs([]slog.Attr{slog.String("a", "b")})

	logger := slog.New(h)
	logger.Info("hello world")

	var record struct {
		Message string `json:"msg"`
		A       string `json:"a"`
	}
	err := json.Unmarshal(buf.Bytes(), &record)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(record.Message)
	fmt.Print(record.A)
	// Output: hello world
	// b
}

func ExampleHandler_WithGroup() {
	var buf bytes.Buffer
	var h slog.Handler = NewHandler(slog.NewJSONHandler(&buf, &slog.HandlerOptions{}))
	h = h.WithGroup("n")

	logger := slog.New(h)
	logger.Info("hello world", slog.Int("one", 1))

	var record struct {
		Message string `json:"msg"`
		N       struct {
			One int `json:"one"`
		} `json:"n"`
	}
	err := json.Unmarshal(buf.Bytes(), &record)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(record.Message)
	fmt.Print(record.N.One)
	// Output: hello world
	// 1
}
