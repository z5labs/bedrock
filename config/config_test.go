// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package config_test

import (
	"context"
	"errors"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/z5labs/bedrock"
	"github.com/z5labs/bedrock/config"

	"github.com/stretchr/testify/require"
)

var (
	_ config.Source[string] = config.Value[string]{}
	_ config.Source[int]    = config.Static(0)
)

// countingEnv is a bedrock.Environment that counts its lookups.
type countingEnv struct {
	env   bedrock.Environment
	calls atomic.Int64
}

func (e *countingEnv) Value(name string) (string, error) {
	e.calls.Add(1)
	return e.env.Value(name)
}

// failingEnv is a bedrock.Environment whose every lookup fails with err.
type failingEnv struct {
	err error
}

func (e failingEnv) Value(string) (string, error) {
	return "", e.err
}

// sourceFunc is a Source that does not import config's own types.
type sourceFunc[T any] func(context.Context) (T, error)

func (f sourceFunc[T]) Read(ctx context.Context) (T, error) {
	return f(ctx)
}

func env(values map[string]string) bedrock.Environment {
	return bedrock.NewStaticEnvironment(values)
}

func TestValue_Read(t *testing.T) {
	t.Parallel()

	t.Run("the zero Value returns ErrNotFound", func(t *testing.T) {
		t.Parallel()

		v, err := config.Value[int]{}.Read(t.Context())
		require.Same(t, config.ErrNotFound, err)
		require.Zero(t, v)
	})

	t.Run("PORT with a parser and a default", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name    string
			env     map[string]string
			want    int
			wantErr string
		}{
			{
				name: "missing reads the default",
				env:  map[string]string{},
				want: 8080,
			},
			{
				name:    "empty is a parse error, not the default",
				env:     map[string]string{"PORT": ""},
				wantErr: `config PORT: strconv.Atoi: parsing "": invalid syntax`,
			},
			{
				name: "valid reads the value",
				env:  map[string]string{"PORT": "9090"},
				want: 9090,
			},
			{
				name:    "malformed is an error, never the default",
				env:     map[string]string{"PORT": "abc"},
				wantErr: `config PORT: strconv.Atoi: parsing "abc": invalid syntax`,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				port, err := config.Env(env(tc.env), "PORT").Parse(strconv.Atoi).Default(8080).Read(t.Context())
				if tc.wantErr != "" {
					require.EqualError(t, err, tc.wantErr)
					require.ErrorIs(t, err, strconv.ErrSyntax)
					require.NotErrorIs(t, err, config.ErrNotFound)
					require.Zero(t, port)
					return
				}
				require.NoError(t, err)
				require.Equal(t, tc.want, port)
			})
		}
	})
}

func TestError(t *testing.T) {
	t.Parallel()

	t.Run("a missing value renders as not found under its name", func(t *testing.T) {
		t.Parallel()

		_, err := config.Env(env(nil), "PORT").Read(t.Context())
		require.EqualError(t, err, "config PORT: not found")
		require.ErrorIs(t, err, config.ErrNotFound)

		cerr, ok := errors.AsType[*config.Error](err)
		require.True(t, ok)
		require.Equal(t, "PORT", cerr.Name)
	})

	t.Run("a parse error is wrapped once", func(t *testing.T) {
		t.Parallel()

		_, err := config.Env(env(map[string]string{"PORT": "abc"}), "PORT").
			Parse(strconv.Atoi).
			Map(func(n int) int { return n + 1 }).
			Read(t.Context())

		cerr, ok := errors.AsType[*config.Error](err)
		require.True(t, ok)
		require.Equal(t, "PORT", cerr.Name)
		_, nested := errors.AsType[*config.Error](cerr.Err)
		require.False(t, nested, "the cause must not be wrapped in a second *config.Error")

		numErr, ok := errors.AsType[*strconv.NumError](err)
		require.True(t, ok)
		require.Equal(t, "abc", numErr.Num)
	})

	t.Run("an error from an unnamed value renders without a name", func(t *testing.T) {
		t.Parallel()

		_, err := config.Static("abc").Parse(strconv.Atoi).Read(t.Context())
		require.EqualError(t, err, `config: strconv.Atoi: parsing "abc": invalid syntax`)
	})

	t.Run("Unwrap returns the cause", func(t *testing.T) {
		t.Parallel()

		cause := errors.New("boom")
		err := &config.Error{Name: "X", Err: cause}
		require.Same(t, cause, errors.Unwrap(err))
		require.EqualError(t, err, "config X: boom")
	})
}

func TestEnv(t *testing.T) {
	t.Parallel()

	t.Run("reads the variable", func(t *testing.T) {
		t.Parallel()

		v, err := config.Env(env(map[string]string{"NAME": "gopher"}), "NAME").Read(t.Context())
		require.NoError(t, err)
		require.Equal(t, "gopher", v)
	})

	t.Run("an empty variable is set", func(t *testing.T) {
		t.Parallel()

		v, err := config.Env(env(map[string]string{"NAME": ""}), "NAME").Default("default").Read(t.Context())
		require.NoError(t, err)
		require.Empty(t, v)
	})

	t.Run("looks the variable up on every Read", func(t *testing.T) {
		t.Parallel()

		e := &countingEnv{env: env(map[string]string{"NAME": "gopher"})}
		v := config.Env(e, "NAME")
		require.Zero(t, e.calls.Load(), "Env must not read at construction")

		for range 3 {
			_, err := v.Read(t.Context())
			require.NoError(t, err)
		}
		require.Equal(t, int64(3), e.calls.Load())
	})

	t.Run("a missing variable is ErrNotFound and a bedrock.ValueNotFoundError", func(t *testing.T) {
		t.Parallel()

		_, err := config.Env(env(nil), "PORT").Read(t.Context())
		require.ErrorIs(t, err, config.ErrNotFound)

		var notFound bedrock.ValueNotFoundError
		require.ErrorAs(t, err, &notFound)
		require.Equal(t, "PORT", notFound.Name)
	})

	t.Run("any other error from the Environment fails the read, even with Default", func(t *testing.T) {
		t.Parallel()

		cause := errors.New("environment unavailable")
		v, err := config.Env(failingEnv{err: cause}, "PORT").Default("8080").Read(t.Context())
		require.ErrorIs(t, err, cause)
		require.NotErrorIs(t, err, config.ErrNotFound)
		require.EqualError(t, err, "config PORT: environment unavailable")
		require.Empty(t, v)
	})

	t.Run("panics on a nil Environment", func(t *testing.T) {
		t.Parallel()

		require.PanicsWithValue(t, "config: Env requires a non-nil bedrock.Environment", func() {
			config.Env(nil, "PORT")
		})
	})
}

func TestValue_Map(t *testing.T) {
	t.Parallel()

	t.Run("transforms the value", func(t *testing.T) {
		t.Parallel()

		v, err := config.Static(2).Map(func(n int) string { return strconv.Itoa(n * 21) }).Read(t.Context())
		require.NoError(t, err)
		require.Equal(t, "42", v)
	})

	t.Run("passes an error through without calling f", func(t *testing.T) {
		t.Parallel()

		called := false
		_, err := config.Env(env(nil), "PORT").Map(func(s string) int {
			called = true
			return len(s)
		}).Read(t.Context())
		require.ErrorIs(t, err, config.ErrNotFound)
		require.EqualError(t, err, "config PORT: not found")
		require.False(t, called)
	})

	t.Run("panics on a nil function", func(t *testing.T) {
		t.Parallel()

		require.PanicsWithValue(t, "config: Map requires a non-nil function", func() {
			config.Static(1).Map[int](nil)
		})
	})
}

func TestValue_Parse(t *testing.T) {
	t.Parallel()

	t.Run("composes with strconv parsers", func(t *testing.T) {
		t.Parallel()

		e := env(map[string]string{"DEBUG": "true", "TIMEOUT": "1.5s"})

		debug, err := config.Env(e, "DEBUG").Parse(strconv.ParseBool).Read(t.Context())
		require.NoError(t, err)
		require.True(t, debug)

		timeout, err := config.Env(e, "TIMEOUT").Parse(time.ParseDuration).Read(t.Context())
		require.NoError(t, err)
		require.Equal(t, 1500*time.Millisecond, timeout)
	})

	t.Run("passes an error through without calling f", func(t *testing.T) {
		t.Parallel()

		called := false
		_, err := config.Env(env(nil), "PORT").Parse(func(s string) (int, error) {
			called = true
			return strconv.Atoi(s)
		}).Read(t.Context())
		require.ErrorIs(t, err, config.ErrNotFound)
		require.False(t, called)
	})

	t.Run("panics on a nil function", func(t *testing.T) {
		t.Parallel()

		require.PanicsWithValue(t, "config: Parse requires a non-nil function", func() {
			config.Static("1").Parse[int](nil)
		})
	})
}

func TestValue_FlatMap(t *testing.T) {
	t.Parallel()

	e := env(map[string]string{
		"DB":          "PRIMARY_DSN",
		"PRIMARY_DSN": "postgres://primary",
	})

	t.Run("reads the Value f returns", func(t *testing.T) {
		t.Parallel()

		dsn, err := config.Env(e, "DB").FlatMap(func(name string) config.Value[string] {
			return config.Env(e, name)
		}).Read(t.Context())
		require.NoError(t, err)
		require.Equal(t, "postgres://primary", dsn)
	})

	t.Run("an error from the returned Value keeps its name", func(t *testing.T) {
		t.Parallel()

		_, err := config.Env(e, "DB").FlatMap(func(string) config.Value[string] {
			return config.Env(e, "MISSING_DSN")
		}).Read(t.Context())
		require.EqualError(t, err, "config MISSING_DSN: not found")
	})

	t.Run("an error from an unnamed returned Value is named after the receiver", func(t *testing.T) {
		t.Parallel()

		_, err := config.Env(e, "DB").FlatMap(func(string) config.Value[string] {
			return config.Value[string]{}
		}).Read(t.Context())
		require.EqualError(t, err, "config DB: not found")
		require.ErrorIs(t, err, config.ErrNotFound)
	})

	t.Run("passes an error through without calling f", func(t *testing.T) {
		t.Parallel()

		called := false
		_, err := config.Env(e, "NOPE").FlatMap(func(string) config.Value[int] {
			called = true
			return config.Static(1)
		}).Read(t.Context())
		require.EqualError(t, err, "config NOPE: not found")
		require.False(t, called)
	})

	t.Run("panics on a nil function", func(t *testing.T) {
		t.Parallel()

		require.PanicsWithValue(t, "config: FlatMap requires a non-nil function", func() {
			config.Static("1").FlatMap[int](nil)
		})
	})
}

var errNegative = errors.New("must not be negative")

// nonNegative is a check for Validate that rejects a negative duration.
func nonNegative(d time.Duration) error {
	if d < 0 {
		return errNegative
	}
	return nil
}

func TestValue_Validate(t *testing.T) {
	t.Parallel()

	e := env(map[string]string{
		"GOOD_TIMEOUT":      "30s",
		"NEGATIVE_TIMEOUT":  "-5s",
		"MALFORMED_TIMEOUT": "30",
	})

	// timeout reads name as a duration and counts the calls of its check.
	timeout := func(name string, calls *atomic.Int64) config.Value[time.Duration] {
		return config.Env(e, name).Parse(time.ParseDuration).Validate(func(d time.Duration) error {
			calls.Add(1)
			return nonNegative(d)
		})
	}

	t.Run("a value that passes f is returned unchanged", func(t *testing.T) {
		t.Parallel()

		var calls atomic.Int64
		d, err := timeout("GOOD_TIMEOUT", &calls).Read(t.Context())
		require.NoError(t, err)
		require.Equal(t, 30*time.Second, d)
		require.EqualValues(t, 1, calls.Load())
	})

	t.Run("a value that fails f is an *Error with the value's name", func(t *testing.T) {
		t.Parallel()

		var calls atomic.Int64
		d, err := timeout("NEGATIVE_TIMEOUT", &calls).Read(t.Context())
		require.Zero(t, d)
		require.EqualError(t, err, "config NEGATIVE_TIMEOUT: must not be negative")
		require.ErrorIs(t, err, errNegative)
		require.NotErrorIs(t, err, config.ErrNotFound)

		cerr, ok := errors.AsType[*config.Error](err)
		require.True(t, ok)
		require.Equal(t, "NEGATIVE_TIMEOUT", cerr.Name)
		require.EqualValues(t, 1, calls.Load())
	})

	t.Run("a value that is not set is not checked and is recovered by Default", func(t *testing.T) {
		t.Parallel()

		var calls atomic.Int64
		_, err := timeout("MISSING_TIMEOUT", &calls).Read(t.Context())
		require.ErrorIs(t, err, config.ErrNotFound)
		require.EqualError(t, err, "config MISSING_TIMEOUT: not found")

		d, err := timeout("MISSING_TIMEOUT", &calls).Default(-time.Second).Read(t.Context())
		require.NoError(t, err)
		require.Equal(t, -time.Second, d, "a Default chained after Validate is not checked")
		require.Zero(t, calls.Load())
	})

	t.Run("a malformed value fails Parse without calling f", func(t *testing.T) {
		t.Parallel()

		var calls atomic.Int64
		_, err := timeout("MALFORMED_TIMEOUT", &calls).Read(t.Context())
		require.EqualError(t, err, "config MALFORMED_TIMEOUT: time: missing unit in duration \"30\"")
		require.Zero(t, calls.Load())
	})

	t.Run("chained after Default, the default is checked under the variable's name", func(t *testing.T) {
		t.Parallel()

		withDefault := func(d time.Duration) config.Value[time.Duration] {
			return config.Env(e, "MISSING_TIMEOUT").Parse(time.ParseDuration).Default(d).Validate(nonNegative)
		}

		_, err := withDefault(-5 * time.Second).Read(t.Context())
		require.EqualError(t, err, "config MISSING_TIMEOUT: must not be negative")
		require.ErrorIs(t, err, errNegative)
		require.NotErrorIs(t, err, config.ErrNotFound)

		d, err := withDefault(5 * time.Second).Read(t.Context())
		require.NoError(t, err)
		require.Equal(t, 5*time.Second, d)
	})

	t.Run("does not check the zero Value, which is not set", func(t *testing.T) {
		t.Parallel()

		called := false
		_, err := config.Value[int]{}.Validate(func(int) error {
			called = true
			return nil
		}).Read(t.Context())
		require.ErrorIs(t, err, config.ErrNotFound)
		require.False(t, called)
	})

	t.Run("panics on a nil function when it is called", func(t *testing.T) {
		t.Parallel()

		require.PanicsWithValue(t, "config: Validate requires a non-nil function", func() {
			config.Static(time.Second).Validate(nil)
		})
	})
}

func TestValue_Or(t *testing.T) {
	t.Parallel()

	t.Run("reads next only when the receiver is not set", func(t *testing.T) {
		t.Parallel()

		e := env(map[string]string{"LEGACY_API_KEY": "legacy", "API_KEY": "current"})

		v, err := config.Env(e, "API_KEY").Or(config.Env(e, "LEGACY_API_KEY")).Read(t.Context())
		require.NoError(t, err)
		require.Equal(t, "current", v)

		v, err = config.Env(e, "NEW_API_KEY").Or(config.Env(e, "API_KEY")).Read(t.Context())
		require.NoError(t, err)
		require.Equal(t, "current", v)
	})

	t.Run("chains from the zero Value", func(t *testing.T) {
		t.Parallel()

		e := env(map[string]string{"C": "c"})
		v, err := config.Value[string]{}.Or(config.Env(e, "A")).Or(config.Env(e, "B")).Or(config.Env(e, "C")).Read(t.Context())
		require.NoError(t, err)
		require.Equal(t, "c", v)
	})

	t.Run("the last missing value is reported under its own name", func(t *testing.T) {
		t.Parallel()

		e := env(nil)
		_, err := config.Env(e, "A").Or(config.Env(e, "B")).Read(t.Context())
		require.EqualError(t, err, "config B: not found")
	})

	t.Run("does not recover from a malformed value", func(t *testing.T) {
		t.Parallel()

		e := env(map[string]string{"PORT": "abc", "FALLBACK_PORT": "9090"})
		_, err := config.Env(e, "PORT").Parse(strconv.Atoi).
			Or(config.Env(e, "FALLBACK_PORT").Parse(strconv.Atoi)).
			Read(t.Context())
		require.EqualError(t, err, `config PORT: strconv.Atoi: parsing "abc": invalid syntax`)
	})

	t.Run("an error from an unnamed Source is named after the receiver", func(t *testing.T) {
		t.Parallel()

		cause := errors.New("vault sealed")
		src := sourceFunc[string](func(context.Context) (string, error) { return "", cause })
		_, err := config.Env(env(nil), "TOKEN").Or(src).Read(t.Context())
		require.EqualError(t, err, "config TOKEN: vault sealed")
		require.ErrorIs(t, err, cause)
	})

	t.Run("panics on a nil Source", func(t *testing.T) {
		t.Parallel()

		require.PanicsWithValue(t, "config: Or requires a non-nil Source", func() {
			config.Static(1).Or(nil)
		})
	})
}

func TestValue_Default(t *testing.T) {
	t.Parallel()

	t.Run("recovers from ErrNotFound reported by any source", func(t *testing.T) {
		t.Parallel()

		v, err := config.Func("FEATURE", func(context.Context) (bool, error) {
			return false, config.ErrNotFound
		}).Default(true).Read(t.Context())
		require.NoError(t, err)
		require.True(t, v)
	})

	t.Run("does not recover from any other error", func(t *testing.T) {
		t.Parallel()

		cause := errors.New("boom")
		_, err := config.Func("FEATURE", func(context.Context) (bool, error) {
			return false, cause
		}).Default(true).Read(t.Context())
		require.EqualError(t, err, "config FEATURE: boom")
	})
}

func TestFunc(t *testing.T) {
	t.Parallel()

	t.Run("calls fn with the context of Read", func(t *testing.T) {
		t.Parallel()

		type ctxKey struct{}
		ctx := context.WithValue(t.Context(), ctxKey{}, "from ctx")
		v, err := config.Func("X", func(ctx context.Context) (string, error) {
			return ctx.Value(ctxKey{}).(string), nil
		}).Read(ctx)
		require.NoError(t, err)
		require.Equal(t, "from ctx", v)
	})

	t.Run("a not found error from fn renders as not found", func(t *testing.T) {
		t.Parallel()

		_, err := config.Func("X", func(context.Context) (int, error) {
			return 0, config.ErrNotFound
		}).Read(t.Context())
		require.EqualError(t, err, "config X: not found")
	})

	t.Run("panics on a nil function", func(t *testing.T) {
		t.Parallel()

		require.PanicsWithValue(t, "config: Func requires a non-nil function", func() {
			config.Func[int]("X", nil)
		})
	})
}

func TestFrom(t *testing.T) {
	t.Parallel()

	t.Run("names a Source and gives it the combinators", func(t *testing.T) {
		t.Parallel()

		src := sourceFunc[string](func(context.Context) (string, error) { return "abc", nil })
		_, err := config.From("SECRET", src).Parse(strconv.Atoi).Read(t.Context())
		require.EqualError(t, err, `config SECRET: strconv.Atoi: parsing "abc": invalid syntax`)
	})

	t.Run("an error from a Value it wraps keeps its name", func(t *testing.T) {
		t.Parallel()

		_, err := config.From("OUTER", config.Env(env(nil), "INNER")).Read(t.Context())
		require.EqualError(t, err, "config INNER: not found")
	})

	t.Run("panics on a nil Source", func(t *testing.T) {
		t.Parallel()

		require.PanicsWithValue(t, "config: From requires a non-nil Source", func() {
			config.From[int]("X", nil)
		})
	})
}

func TestStatic(t *testing.T) {
	t.Parallel()

	v, err := config.Static(42).Read(t.Context())
	require.NoError(t, err)
	require.Equal(t, 42, v)
}

func TestParsers(t *testing.T) {
	t.Parallel()

	e := env(map[string]string{
		"MAX_BYTES": "9223372036854775807",
		"RATIO":     "0.25",
		"LEVEL":     "info",
		"BAD_LEVEL": "trace",
		"BAD_INT":   "1.5",
		"BAD_FLOAT": "x",
	})

	t.Run("ParseInt64", func(t *testing.T) {
		t.Parallel()

		n, err := config.Env(e, "MAX_BYTES").Parse(config.ParseInt64).Read(t.Context())
		require.NoError(t, err)
		require.Equal(t, int64(9223372036854775807), n)

		_, err = config.Env(e, "BAD_INT").Parse(config.ParseInt64).Read(t.Context())
		require.EqualError(t, err, `config BAD_INT: strconv.ParseInt: parsing "1.5": invalid syntax`)
	})

	t.Run("ParseFloat64", func(t *testing.T) {
		t.Parallel()

		f, err := config.Env(e, "RATIO").Parse(config.ParseFloat64).Read(t.Context())
		require.NoError(t, err)
		require.Equal(t, 0.25, f)

		_, err = config.Env(e, "BAD_FLOAT").Parse(config.ParseFloat64).Read(t.Context())
		require.EqualError(t, err, `config BAD_FLOAT: strconv.ParseFloat: parsing "x": invalid syntax`)
	})

	t.Run("OneOf", func(t *testing.T) {
		t.Parallel()

		allowed := []string{"debug", "info"}
		oneOf := config.OneOf(allowed...)
		allowed[1] = "trace" // OneOf keeps its own copy

		level, err := config.Env(e, "LEVEL").Parse(oneOf).Read(t.Context())
		require.NoError(t, err)
		require.Equal(t, "info", level)

		_, err = config.Env(e, "BAD_LEVEL").Parse(oneOf).Read(t.Context())
		require.EqualError(t, err, "config BAD_LEVEL: trace is not one of [debug info]")

		_, err = config.Env(e, "MISSING_LEVEL").Parse(oneOf).Default("info").Read(t.Context())
		require.NoError(t, err)
	})

	t.Run("OneOf composes after another parser", func(t *testing.T) {
		t.Parallel()

		n, err := config.Static("2").Parse(strconv.Atoi).Parse(config.OneOf(1, 2, 4)).Read(t.Context())
		require.NoError(t, err)
		require.Equal(t, 2, n)
	})

	t.Run("OneOf panics with nothing allowed", func(t *testing.T) {
		t.Parallel()

		require.PanicsWithValue(t, "config: OneOf requires at least one allowed value", func() {
			config.OneOf[string]()
		})
	})
}
