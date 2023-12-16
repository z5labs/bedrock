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

// Bools
func Bools(key string, values []bool) slog.Attr {
	return slog.Any(key, values)
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

// Strings
func Strings(key string, values []string) slog.Attr {
	return slog.Any(key, values)
}

// Int
func Int(key string, n int) slog.Attr {
	return slog.Int(key, n)
}

// Ints
func Ints(key string, ns []int) slog.Attr {
	return slog.Any(key, ns)
}

// Int8
func Int8(key string, n int8) slog.Attr {
	return slog.Int64(key, int64(n))
}

// Int8s
func Int8s(key string, ns []int8) slog.Attr {
	return slog.Any(key, ns)
}

// Int16
func Int16(key string, n int16) slog.Attr {
	return slog.Int64(key, int64(n))
}

// Int16s
func Int16s(key string, ns []int16) slog.Attr {
	return slog.Any(key, ns)
}

// Int32
func Int32(key string, n int32) slog.Attr {
	return slog.Int64(key, int64(n))
}

// Int32s
func Int32s(key string, ns []int32) slog.Attr {
	return slog.Any(key, ns)
}

// Int64
func Int64(key string, n int64) slog.Attr {
	return slog.Int64(key, n)
}

// Int64s
func Int64s(key string, ns []int64) slog.Attr {
	return slog.Any(key, ns)
}

// Uint
func Uint(key string, n uint) slog.Attr {
	return slog.Uint64(key, uint64(n))
}

// Uints
func Uints(key string, ns []uint) slog.Attr {
	return slog.Any(key, ns)
}

// Uint8
func Uint8(key string, n uint8) slog.Attr {
	return slog.Uint64(key, uint64(n))
}

// Uint8s
func Uint8s(key string, ns []uint8) slog.Attr {
	return slog.Any(key, ns)
}

// Uint16
func Uint16(key string, n uint16) slog.Attr {
	return slog.Uint64(key, uint64(n))
}

// Uint16s
func Uint16s(key string, ns []uint16) slog.Attr {
	return slog.Any(key, ns)
}

// Uint32
func Uint32(key string, n uint32) slog.Attr {
	return slog.Uint64(key, uint64(n))
}

// Uint32s
func Uint32s(key string, ns []uint32) slog.Attr {
	return slog.Any(key, ns)
}

// Uint64
func Uint64(key string, n uint64) slog.Attr {
	return slog.Uint64(key, n)
}

// Uint64s
func Uint64s(key string, ns []uint64) slog.Attr {
	return slog.Any(key, ns)
}
