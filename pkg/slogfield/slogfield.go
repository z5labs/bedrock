// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package slogfield

import (
	"log/slog"
	"time"
)

// Any returns an slog.Attr for the supplied value.
func Any(key string, value any) slog.Attr {
	return slog.Any(key, value)
}

// Bool returns an slog.Attr for a bool.
func Bool(key string, value bool) slog.Attr {
	return slog.Bool(key, value)
}

// Bools returns an slog.Attr for a slice of bools.
func Bools(key string, values []bool) slog.Attr {
	return slog.Any(key, values)
}

// Duration returns an slog.Attr for a time.Duration.
func Duration(key string, d time.Duration) slog.Attr {
	return slog.Duration(key, d)
}

// Error returns an slog.Attr for a error.
func Error(err error) slog.Attr {
	return slog.Any("error", err)
}

// String returns an slog.Attr for a string.
func String(key, value string) slog.Attr {
	return slog.String(key, value)
}

// Strings returns an slog.Attr for a slice of strings.
func Strings(key string, values []string) slog.Attr {
	return slog.Any(key, values)
}

// Int returns an slog.Attr for a int.
func Int(key string, n int) slog.Attr {
	return slog.Int(key, n)
}

// Ints returns an slog.Attr for a slice of ints.
func Ints(key string, ns []int) slog.Attr {
	return slog.Any(key, ns)
}

// Int8 returns an slog.Attr for a int8.
func Int8(key string, n int8) slog.Attr {
	return slog.Int64(key, int64(n))
}

// Int8s returns an slog.Attr for a slice of int8s.
func Int8s(key string, ns []int8) slog.Attr {
	return slog.Any(key, ns)
}

// Int16 returns an slog.Attr for a int16.
func Int16(key string, n int16) slog.Attr {
	return slog.Int64(key, int64(n))
}

// Int16s returns an slog.Attr for a slice of int16s.
func Int16s(key string, ns []int16) slog.Attr {
	return slog.Any(key, ns)
}

// Int32 returns an slog.Attr for a int32.
func Int32(key string, n int32) slog.Attr {
	return slog.Int64(key, int64(n))
}

// Int32s returns an slog.Attr for a slice of int32s.
func Int32s(key string, ns []int32) slog.Attr {
	return slog.Any(key, ns)
}

// Int64 returns an slog.Attr for a int64.
func Int64(key string, n int64) slog.Attr {
	return slog.Int64(key, n)
}

// Int64s returns an slog.Attr for a slice of int64s.
func Int64s(key string, ns []int64) slog.Attr {
	return slog.Any(key, ns)
}

// Uint returns an slog.Attr for a uint.
func Uint(key string, n uint) slog.Attr {
	return slog.Uint64(key, uint64(n))
}

// Uints returns an slog.Attr for a slice of uints.
func Uints(key string, ns []uint) slog.Attr {
	return slog.Any(key, ns)
}

// Uint8 returns an slog.Attr for a uint8.
func Uint8(key string, n uint8) slog.Attr {
	return slog.Uint64(key, uint64(n))
}

// Uint8s returns an slog.Attr for a uint8s.
func Uint8s(key string, ns []uint8) slog.Attr {
	return slog.Any(key, ns)
}

// Uint16 returns an slog.Attr for a uint16.
func Uint16(key string, n uint16) slog.Attr {
	return slog.Uint64(key, uint64(n))
}

// Uint16s returns an slog.Attr for a uint16s.
func Uint16s(key string, ns []uint16) slog.Attr {
	return slog.Any(key, ns)
}

// Uint32 returns an slog.Attr for a uint32.
func Uint32(key string, n uint32) slog.Attr {
	return slog.Uint64(key, uint64(n))
}

// Uint32s returns an slog.Attr for a slice of uint32s.
func Uint32s(key string, ns []uint32) slog.Attr {
	return slog.Any(key, ns)
}

// Uint64 returns an slog.Attr for a uint64.
func Uint64(key string, n uint64) slog.Attr {
	return slog.Uint64(key, n)
}

// Uint64s returns an slog.Attr for a slice of uint64s.
func Uint64s(key string, ns []uint64) slog.Attr {
	return slog.Any(key, ns)
}
