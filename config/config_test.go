// Copyright (c) 2025 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package config

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestValue_Value(t *testing.T) {
	testCases := []struct {
		name        string
		value       Value[int]
		expectedVal int
		expectedOk  bool
	}{
		{
			name:        "set value",
			value:       ValueOf(42),
			expectedVal: 42,
			expectedOk:  true,
		},
		{
			name:        "unset value",
			value:       Value[int]{},
			expectedVal: 0,
			expectedOk:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			val, ok := tc.value.Value()
			require.Equal(t, tc.expectedOk, ok)
			require.Equal(t, tc.expectedVal, val)
		})
	}
}

func TestRead(t *testing.T) {
	testCases := []struct {
		name        string
		reader      Reader[string]
		expectedVal string
		expectErr   error
	}{
		{
			name: "returns value when set",
			reader: ReaderFunc[string](func(ctx context.Context) (Value[string], error) {
				return ValueOf("test"), nil
			}),
			expectedVal: "test",
		},
		{
			name: "returns error when reader fails",
			reader: ReaderFunc[string](func(ctx context.Context) (Value[string], error) {
				return Value[string]{}, errors.New("read failed")
			}),
			expectErr: errors.New("read failed"),
		},
		{
			name: "returns error when value not set",
			reader: ReaderFunc[string](func(ctx context.Context) (Value[string], error) {
				return Value[string]{}, nil
			}),
			expectErr: ErrValueNotSet,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			val, err := Read(context.Background(), tc.reader)
			if tc.expectErr != nil {
				require.Error(t, err)
				if tc.expectErr == ErrValueNotSet {
					require.ErrorIs(t, err, ErrValueNotSet)
				}
				require.Zero(t, val)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expectedVal, val)
			}
		})
	}
}

func TestMust(t *testing.T) {
	testCases := []struct {
		name        string
		reader      Reader[int]
		expectedVal int
		expectPanic bool
	}{
		{
			name: "returns value when set",
			reader: ReaderFunc[int](func(ctx context.Context) (Value[int], error) {
				return ValueOf(123), nil
			}),
			expectedVal: 123,
		},
		{
			name: "panics on error",
			reader: ReaderFunc[int](func(ctx context.Context) (Value[int], error) {
				return Value[int]{}, errors.New("read failed")
			}),
			expectPanic: true,
		},
		{
			name: "panics when value not set",
			reader: ReaderFunc[int](func(ctx context.Context) (Value[int], error) {
				return Value[int]{}, nil
			}),
			expectPanic: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.expectPanic {
				require.Panics(t, func() {
					Must(context.Background(), tc.reader)
				})
			} else {
				val := Must(context.Background(), tc.reader)
				require.Equal(t, tc.expectedVal, val)
			}
		})
	}
}

func TestMustOr(t *testing.T) {
	testCases := []struct {
		name         string
		reader       Reader[int]
		defaultValue int
		expectedVal  int
	}{
		{
			name: "returns value when set",
			reader: ReaderFunc[int](func(ctx context.Context) (Value[int], error) {
				return ValueOf(42), nil
			}),
			defaultValue: 99,
			expectedVal:  42,
		},
		{
			name: "returns default when value not set",
			reader: ReaderFunc[int](func(ctx context.Context) (Value[int], error) {
				return Value[int]{}, nil
			}),
			defaultValue: 99,
			expectedVal:  99,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			val := MustOr(context.Background(), tc.defaultValue, tc.reader)
			require.Equal(t, tc.expectedVal, val)
		})
	}
}

func TestDefault(t *testing.T) {
	testCases := []struct {
		name         string
		reader       Reader[string]
		defaultValue string
		expectedVal  string
		expectErr    bool
	}{
		{
			name: "returns original value when set",
			reader: ReaderFunc[string](func(ctx context.Context) (Value[string], error) {
				return ValueOf("original"), nil
			}),
			defaultValue: "default",
			expectedVal:  "original",
		},
		{
			name: "returns default when value not set",
			reader: ReaderFunc[string](func(ctx context.Context) (Value[string], error) {
				return Value[string]{}, nil
			}),
			defaultValue: "default",
			expectedVal:  "default",
		},
		{
			name: "propagates error",
			reader: ReaderFunc[string](func(ctx context.Context) (Value[string], error) {
				return Value[string]{}, errors.New("read failed")
			}),
			defaultValue: "default",
			expectErr:    true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dr := Default(tc.defaultValue, tc.reader)
			val, err := Read(context.Background(), dr)
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expectedVal, val)
			}
		})
	}
}

func TestOr(t *testing.T) {
	testCases := []struct {
		name        string
		readers     []Reader[int]
		expectedVal int
		expectSet   bool
		expectErr   bool
	}{
		{
			name: "returns first set value",
			readers: []Reader[int]{
				ReaderFunc[int](func(ctx context.Context) (Value[int], error) {
					return Value[int]{}, nil
				}),
				ReaderFunc[int](func(ctx context.Context) (Value[int], error) {
					return ValueOf(42), nil
				}),
				ReaderFunc[int](func(ctx context.Context) (Value[int], error) {
					return ValueOf(99), nil
				}),
			},
			expectedVal: 42,
			expectSet:   true,
		},
		{
			name: "returns unset when no readers have value",
			readers: []Reader[int]{
				ReaderFunc[int](func(ctx context.Context) (Value[int], error) {
					return Value[int]{}, nil
				}),
				ReaderFunc[int](func(ctx context.Context) (Value[int], error) {
					return Value[int]{}, nil
				}),
			},
			expectSet: false,
		},
		{
			name: "propagates error",
			readers: []Reader[int]{
				ReaderFunc[int](func(ctx context.Context) (Value[int], error) {
					return Value[int]{}, nil
				}),
				ReaderFunc[int](func(ctx context.Context) (Value[int], error) {
					return Value[int]{}, errors.New("read failed")
				}),
			},
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			or := Or(tc.readers...)
			val, err := or.Read(context.Background())
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				v, ok := val.Value()
				require.Equal(t, tc.expectSet, ok)
				if tc.expectSet {
					require.Equal(t, tc.expectedVal, v)
				}
			}
		})
	}
}

func TestMap(t *testing.T) {
	testCases := []struct {
		name        string
		reader      Reader[string]
		mapper      func(context.Context, string) (int, error)
		expectedVal int
		expectSet   bool
		expectErr   bool
	}{
		{
			name: "maps value when set",
			reader: ReaderFunc[string](func(ctx context.Context) (Value[string], error) {
				return ValueOf("42"), nil
			}),
			mapper: func(ctx context.Context, s string) (int, error) {
				return 42, nil
			},
			expectedVal: 42,
			expectSet:   true,
		},
		{
			name: "returns unset when reader returns unset",
			reader: ReaderFunc[string](func(ctx context.Context) (Value[string], error) {
				return Value[string]{}, nil
			}),
			mapper: func(ctx context.Context, s string) (int, error) {
				return 42, nil
			},
			expectSet: false,
		},
		{
			name: "propagates reader error",
			reader: ReaderFunc[string](func(ctx context.Context) (Value[string], error) {
				return Value[string]{}, errors.New("read failed")
			}),
			mapper: func(ctx context.Context, s string) (int, error) {
				return 42, nil
			},
			expectErr: true,
		},
		{
			name: "propagates mapper error",
			reader: ReaderFunc[string](func(ctx context.Context) (Value[string], error) {
				return ValueOf("test"), nil
			}),
			mapper: func(ctx context.Context, s string) (int, error) {
				return 0, errors.New("map failed")
			},
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mr := Map(tc.reader, tc.mapper)
			val, err := mr.Read(context.Background())
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				v, ok := val.Value()
				require.Equal(t, tc.expectSet, ok)
				if tc.expectSet {
					require.Equal(t, tc.expectedVal, v)
				}
			}
		})
	}
}

func TestBind(t *testing.T) {
	testCases := []struct {
		name        string
		reader      Reader[string]
		binder      func(context.Context, string) Reader[int]
		expectedVal int
		expectSet   bool
		expectErr   bool
	}{
		{
			name: "binds value when set",
			reader: ReaderFunc[string](func(ctx context.Context) (Value[string], error) {
				return ValueOf("key"), nil
			}),
			binder: func(ctx context.Context, key string) Reader[int] {
				return ReaderOf(42)
			},
			expectedVal: 42,
			expectSet:   true,
		},
		{
			name: "returns unset when reader returns unset",
			reader: ReaderFunc[string](func(ctx context.Context) (Value[string], error) {
				return Value[string]{}, nil
			}),
			binder: func(ctx context.Context, key string) Reader[int] {
				return ReaderOf(42)
			},
			expectSet: false,
		},
		{
			name: "propagates reader error",
			reader: ReaderFunc[string](func(ctx context.Context) (Value[string], error) {
				return Value[string]{}, errors.New("read failed")
			}),
			binder: func(ctx context.Context, key string) Reader[int] {
				return ReaderOf(42)
			},
			expectErr: true,
		},
		{
			name: "propagates bound reader error",
			reader: ReaderFunc[string](func(ctx context.Context) (Value[string], error) {
				return ValueOf("key"), nil
			}),
			binder: func(ctx context.Context, key string) Reader[int] {
				return ReaderFunc[int](func(ctx context.Context) (Value[int], error) {
					return Value[int]{}, errors.New("bind failed")
				})
			},
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			br := Bind(tc.reader, tc.binder)
			val, err := br.Read(context.Background())
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				v, ok := val.Value()
				require.Equal(t, tc.expectSet, ok)
				if tc.expectSet {
					require.Equal(t, tc.expectedVal, v)
				}
			}
		})
	}
}

func TestReaderOf(t *testing.T) {
	testCases := []struct {
		name        string
		value       string
		expectedVal string
	}{
		{
			name:        "returns given value",
			value:       "test",
			expectedVal: "test",
		},
		{
			name:        "returns empty string",
			value:       "",
			expectedVal: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := ReaderOf(tc.value)
			val, err := Read(context.Background(), r)
			require.NoError(t, err)
			require.Equal(t, tc.expectedVal, val)
		})
	}
}

func TestEnv(t *testing.T) {
	testCases := []struct {
		name        string
		envKey      string
		envValue    string
		setEnv      bool
		expectedVal string
		expectSet   bool
	}{
		{
			name:        "returns environment variable when set",
			envKey:      "TEST_ENV_VAR_SET",
			envValue:    "test_value",
			setEnv:      true,
			expectedVal: "test_value",
			expectSet:   true,
		},
		{
			name:      "returns unset when variable not set",
			envKey:    "TEST_ENV_VAR_UNSET",
			expectSet: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.setEnv {
				os.Setenv(tc.envKey, tc.envValue)
				defer os.Unsetenv(tc.envKey)
			}

			r := Env(tc.envKey)
			val, err := r.Read(context.Background())
			require.NoError(t, err)
			v, ok := val.Value()
			require.Equal(t, tc.expectSet, ok)
			if tc.expectSet {
				require.Equal(t, tc.expectedVal, v)
			}
		})
	}
}

func TestBoolFromString(t *testing.T) {
	testCases := []struct {
		name        string
		input       string
		expectedVal bool
		expectErr   bool
	}{
		{
			name:        "parses true",
			input:       "true",
			expectedVal: true,
		},
		{
			name:        "parses false",
			input:       "false",
			expectedVal: false,
		},
		{
			name:        "parses 1 as true",
			input:       "1",
			expectedVal: true,
		},
		{
			name:        "parses 0 as false",
			input:       "0",
			expectedVal: false,
		},
		{
			name:      "errors on invalid bool",
			input:     "not-a-bool",
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := BoolFromString(ReaderOf(tc.input))
			val, err := Read(context.Background(), r)
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expectedVal, val)
			}
		})
	}
}

func TestIntFromString(t *testing.T) {
	testCases := []struct {
		name        string
		input       string
		expectedVal int
		expectErr   bool
	}{
		{
			name:        "parses positive int",
			input:       "42",
			expectedVal: 42,
		},
		{
			name:        "parses negative int",
			input:       "-42",
			expectedVal: -42,
		},
		{
			name:        "parses zero",
			input:       "0",
			expectedVal: 0,
		},
		{
			name:      "errors on invalid int",
			input:     "not-an-int",
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := IntFromString(ReaderOf(tc.input))
			val, err := Read(context.Background(), r)
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expectedVal, val)
			}
		})
	}
}

func TestInt64FromBytes(t *testing.T) {
	testCases := []struct {
		name        string
		endian      binary.ByteOrder
		setupBytes  func() []byte
		expectedVal int64
		expectErr   bool
	}{
		{
			name:   "reads little endian int64",
			endian: binary.LittleEndian,
			setupBytes: func() []byte {
				buf := make([]byte, 8)
				binary.LittleEndian.PutUint64(buf, 12345)
				return buf
			},
			expectedVal: 12345,
		},
		{
			name:   "reads big endian int64",
			endian: binary.BigEndian,
			setupBytes: func() []byte {
				buf := make([]byte, 8)
				binary.BigEndian.PutUint64(buf, 67890)
				return buf
			},
			expectedVal: 67890,
		},
		{
			name:   "errors on empty reader",
			endian: binary.LittleEndian,
			setupBytes: func() []byte {
				return []byte{}
			},
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			buf := tc.setupBytes()
			r := Int64FromBytes(tc.endian, ReaderOf(bytes.NewReader(buf)))
			val, err := Read(context.Background(), r)
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expectedVal, val)
			}
		})
	}
}

func TestInt64FromString(t *testing.T) {
	testCases := []struct {
		name        string
		input       string
		expectedVal int64
		expectErr   bool
	}{
		{
			name:        "parses max int64",
			input:       "9223372036854775807",
			expectedVal: 9223372036854775807,
		},
		{
			name:        "parses min int64",
			input:       "-9223372036854775808",
			expectedVal: -9223372036854775808,
		},
		{
			name:      "errors on invalid int64",
			input:     "not-an-int64",
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := Int64FromString(ReaderOf(tc.input))
			val, err := Read(context.Background(), r)
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expectedVal, val)
			}
		})
	}
}

func TestFloat64FromString(t *testing.T) {
	testCases := []struct {
		name        string
		input       string
		expectedVal float64
		expectErr   bool
	}{
		{
			name:        "parses float",
			input:       "3.14159",
			expectedVal: 3.14159,
		},
		{
			name:        "parses negative float",
			input:       "-2.71828",
			expectedVal: -2.71828,
		},
		{
			name:        "parses scientific notation",
			input:       "1.23e10",
			expectedVal: 1.23e10,
		},
		{
			name:      "errors on invalid float",
			input:     "not-a-float",
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := Float64FromString(ReaderOf(tc.input))
			val, err := Read(context.Background(), r)
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.InDelta(t, tc.expectedVal, val, 0.00001)
			}
		})
	}
}

func TestDurationFromString(t *testing.T) {
	testCases := []struct {
		name        string
		input       string
		expectedVal time.Duration
		expectErr   bool
	}{
		{
			name:        "parses seconds",
			input:       "5s",
			expectedVal: 5 * time.Second,
		},
		{
			name:        "parses minutes",
			input:       "10m",
			expectedVal: 10 * time.Minute,
		},
		{
			name:        "parses hours",
			input:       "2h",
			expectedVal: 2 * time.Hour,
		},
		{
			name:        "parses complex duration",
			input:       "1h30m45s",
			expectedVal: time.Hour + 30*time.Minute + 45*time.Second,
		},
		{
			name:      "errors on invalid duration",
			input:     "not-a-duration",
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := DurationFromString(ReaderOf(tc.input))
			val, err := Read(context.Background(), r)
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expectedVal, val)
			}
		})
	}
}

func TestReadFile(t *testing.T) {
	t.Run("reads existing file", func(t *testing.T) {
		tmpFile, err := os.CreateTemp("", "test-*.txt")
		require.NoError(t, err)
		defer os.Remove(tmpFile.Name())
		tmpFile.Close()

		r := ReadFile(tmpFile.Name())
		f, err := Read(context.Background(), r)
		require.NoError(t, err)
		require.NotNil(t, f)
		f.Close()
	})

	t.Run("returns unset for non-existent file", func(t *testing.T) {
		r := ReadFile("/non/existent/file/path/xyz.txt")
		val, err := r.Read(context.Background())
		require.NoError(t, err)
		_, ok := val.Value()
		require.False(t, ok)
	})

	t.Run("errors on permission denied", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "test-dir-*")
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)

		restrictedPath := tmpDir + "/restricted"
		err = os.Mkdir(restrictedPath, 0000)
		require.NoError(t, err)

		r := ReadFile(restrictedPath + "/file.txt")
		val, err := Read(context.Background(), r)
		require.Error(t, err)
		require.Nil(t, val)
	})
}
