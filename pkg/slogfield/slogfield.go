// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package slogfield re-exports and provides extra slog.Attrs.
package slogfield

import (
	"log/slog"
	"time"
)

// Any
func Any(key string, value any) slog.Attr {
	return slog.Any(key, value)
}

// Bool
func Bool(key string, value bool) slog.Attr {
	return slog.Bool(key, value)
}

// Duration
func Duration(key string, d time.Duration) slog.Attr {
	return slog.Duration(key, d)
}

// Error
func Error(err error) slog.Attr {
	return slog.Any("error", err)
}

// String
func String(key, value string) slog.Attr {
	return slog.String(key, value)
}

// Int
func Int(key string, n int) slog.Attr {
	return slog.Int(key, n)
}

// Int8
func Int8(key string, n int8) slog.Attr {
	return slog.Int64(key, int64(n))
}

// Int8
func Int16(key string, n int16) slog.Attr {
	return slog.Int64(key, int64(n))
}

// Int32
func Int32(key string, n int32) slog.Attr {
	return slog.Int64(key, int64(n))
}

// Int64
func Int64(key string, n int64) slog.Attr {
	return slog.Int64(key, n)
}

// Uint
func Uint(key string, n uint) slog.Attr {
	return slog.Uint64(key, uint64(n))
}

// Uint8
func Uint8(key string, n uint8) slog.Attr {
	return slog.Uint64(key, uint64(n))
}

// Uint16
func Uint16(key string, n uint16) slog.Attr {
	return slog.Uint64(key, uint64(n))
}

// Uint32
func Uint32(key string, n uint32) slog.Attr {
	return slog.Uint64(key, uint64(n))
}

// Uint64
func Uint64(key string, n uint64) slog.Attr {
	return slog.Uint64(key, n)
}
