// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package config_test

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/z5labs/bedrock"
	"github.com/z5labs/bedrock/config"

	"github.com/stretchr/testify/require"
)

type settings struct {
	Addr        string
	ReadTimeout time.Duration
	Mode        string
}

func settingsOf(e bedrock.Environment) config.Value[settings] {
	return config.Struct(func(r *config.Resolver) settings {
		env := func(name string) config.Value[string] { return config.Env(e, name) }
		return settings{
			Addr:        r.Get(env("ADDR")),
			ReadTimeout: r.Get(env("READ_TIMEOUT").Parse(time.ParseDuration).Default(5 * time.Second)),
			Mode:        r.Get(env("MODE").Parse(config.OneOf("dev", "prod")).Default("dev")),
		}
	})
}

func TestStruct(t *testing.T) {
	t.Parallel()

	t.Run("every value set builds the struct", func(t *testing.T) {
		t.Parallel()

		got, err := settingsOf(env(map[string]string{
			"ADDR":         ":8080",
			"READ_TIMEOUT": "30s",
			"MODE":         "prod",
		})).Read(t.Context())
		require.NoError(t, err)
		require.Equal(t, settings{Addr: ":8080", ReadTimeout: 30 * time.Second, Mode: "prod"}, got)
	})

	t.Run("defaults fill values that are not set", func(t *testing.T) {
		t.Parallel()

		got, err := settingsOf(env(map[string]string{"ADDR": ":8080"})).Read(t.Context())
		require.NoError(t, err)
		require.Equal(t, settings{Addr: ":8080", ReadTimeout: 5 * time.Second, Mode: "dev"}, got)
	})

	t.Run("every failure is reported in one error", func(t *testing.T) {
		t.Parallel()

		got, err := settingsOf(env(map[string]string{
			"READ_TIMEOUT": "30",
			"MODE":         "staging",
		})).Read(t.Context())
		require.Zero(t, got)
		require.EqualError(t, err, "config ADDR: not found\n"+
			"config READ_TIMEOUT: time: missing unit in duration \"30\"\n"+
			"config MODE: staging is not one of [dev prod]")
		require.ErrorIs(t, err, config.ErrNotFound)

		joined, ok := err.(interface{ Unwrap() []error })
		require.True(t, ok, "the error is an errors.Join of every failure")

		var names []string
		for _, e := range joined.Unwrap() {
			cerr, ok := errors.AsType[*config.Error](e)
			require.True(t, ok, "each failure is a *config.Error")
			names = append(names, cerr.Name)
		}
		require.Equal(t, []string{"ADDR", "READ_TIMEOUT", "MODE"}, names)

		cerr, ok := errors.AsType[*config.Error](err)
		require.True(t, ok)
		require.Equal(t, "ADDR", cerr.Name)
	})

	t.Run("a single failure returns the zero struct", func(t *testing.T) {
		t.Parallel()

		got, err := settingsOf(env(map[string]string{
			"ADDR":         ":8080",
			"READ_TIMEOUT": "abc",
		})).Read(t.Context())
		require.Zero(t, got)
		require.EqualError(t, err, `config READ_TIMEOUT: time: invalid duration "abc"`)
		_, ok := errors.AsType[bedrock.ValueNotFoundError](err)
		require.False(t, ok)
	})

	t.Run("every Get is attempted after one fails", func(t *testing.T) {
		t.Parallel()

		var calls []string
		read := func(name string, err error) config.Value[int] {
			return config.Func(name, func(context.Context) (int, error) {
				calls = append(calls, name)
				return len(name), err
			})
		}

		boom := errors.New("boom")
		got, err := config.Struct(func(r *config.Resolver) []int {
			return []int{
				r.Get(read("A", boom)),
				r.Get(read("BB", nil)),
				r.Get(read("CCC", config.ErrNotFound)),
				r.Get(read("DDDD", nil)),
			}
		}).Read(t.Context())
		require.Equal(t, []string{"A", "BB", "CCC", "DDDD"}, calls)
		require.Nil(t, got)
		require.ErrorIs(t, err, boom)
		require.ErrorIs(t, err, config.ErrNotFound)
		require.EqualError(t, err, "config A: boom\nconfig CCC: not found")
	})

	t.Run("a failed Get returns the zero value to build", func(t *testing.T) {
		t.Parallel()

		var seen []int
		_, err := config.Struct(func(r *config.Resolver) int {
			n := r.Get(config.Env(env(map[string]string{"N": "x"}), "N").Parse(strconv.Atoi))
			seen = append(seen, n)
			return n
		}).Read(t.Context())
		require.Error(t, err)
		require.Equal(t, []int{0}, seen)
	})

	t.Run("an unnamed source failure is named once", func(t *testing.T) {
		t.Parallel()

		boom := errors.New("boom")
		_, err := config.Struct(func(r *config.Resolver) int {
			return r.Get(sourceFunc[int](func(context.Context) (int, error) {
				return 0, boom
			}))
		}).Read(t.Context())
		require.EqualError(t, err, "config: boom")
		require.ErrorIs(t, err, boom)

		cerr, ok := errors.AsType[*config.Error](err)
		require.True(t, ok)
		require.Empty(t, cerr.Name)
		require.Same(t, boom, cerr.Err)
	})

	t.Run("the zero Value is not found", func(t *testing.T) {
		t.Parallel()

		_, err := config.Struct(func(r *config.Resolver) int {
			return r.Get(config.Value[int]{})
		}).Read(t.Context())
		require.ErrorIs(t, err, config.ErrNotFound)
		require.EqualError(t, err, "config: not found")
	})

	t.Run("Get reads with the context of Read", func(t *testing.T) {
		t.Parallel()

		type key struct{}
		ctx := context.WithValue(t.Context(), key{}, "v")
		got, err := config.Struct(func(r *config.Resolver) any {
			return r.Get(config.Func("CTX", func(ctx context.Context) (any, error) {
				return ctx.Value(key{}), nil
			}))
		}).Read(ctx)
		require.NoError(t, err)
		require.Equal(t, "v", got)
	})

	t.Run("every Read builds again with a fresh Resolver", func(t *testing.T) {
		t.Parallel()

		ce := &countingEnv{env: env(map[string]string{})}
		v := config.Struct(func(r *config.Resolver) string {
			return r.Get(config.Env(ce, "ADDR"))
		})

		_, err := v.Read(t.Context())
		require.ErrorIs(t, err, config.ErrNotFound)
		_, err = v.Read(t.Context())
		require.EqualError(t, err, "config ADDR: not found", "the failure of an earlier Read is not carried over")
		require.Equal(t, int64(2), ce.calls.Load())
	})

	t.Run("Get is safe for concurrent use within build", func(t *testing.T) {
		t.Parallel()

		got, err := config.Struct(func(r *config.Resolver) []int {
			out := make([]int, 8)
			var wg sync.WaitGroup
			for i := range out {
				wg.Go(func() {
					if i%2 == 0 {
						out[i] = r.Get(config.Static(i))
						return
					}
					out[i] = r.Get(config.Value[int]{})
				})
			}
			wg.Wait()
			return out
		}).Read(t.Context())
		require.Nil(t, got)
		require.ErrorIs(t, err, config.ErrNotFound)

		joined, ok := err.(interface{ Unwrap() []error })
		require.True(t, ok)
		require.Len(t, joined.Unwrap(), 4)
	})

	t.Run("composes with the other combinators", func(t *testing.T) {
		t.Parallel()

		type db struct{ DSN string }
		type app struct {
			DB   db
			Port int
		}
		e := env(map[string]string{"DB": "REPLICA_DSN", "REPLICA_DSN": "postgres://replica"})
		dbv := config.Struct(func(r *config.Resolver) db {
			return db{DSN: r.Get(config.Env(e, "DB").FlatMap(func(name string) config.Value[string] {
				return config.Env(e, name)
			}))}
		})
		got, err := config.Struct(func(r *config.Resolver) app {
			return app{
				DB:   r.Get(dbv),
				Port: r.Get(config.Env(e, "PORT").Parse(strconv.Atoi).Default(8080)),
			}
		}).Read(t.Context())
		require.NoError(t, err)
		require.Equal(t, app{DB: db{DSN: "postgres://replica"}, Port: 8080}, got)
	})

	t.Run("two values that fail Validate are reported together", func(t *testing.T) {
		t.Parallel()

		type timeouts struct {
			Read  time.Duration
			Write time.Duration
			Idle  time.Duration
		}
		e := env(map[string]string{"READ_TIMEOUT": "-5s", "IDLE_TIMEOUT": "1m"})
		timeout := func(name string, d time.Duration) config.Value[time.Duration] {
			return config.Env(e, name).Parse(time.ParseDuration).Default(d).Validate(nonNegative)
		}

		got, err := config.Struct(func(r *config.Resolver) timeouts {
			return timeouts{
				Read:  r.Get(timeout("READ_TIMEOUT", 5*time.Second)),
				Write: r.Get(timeout("WRITE_TIMEOUT", -time.Second)),
				Idle:  r.Get(timeout("IDLE_TIMEOUT", time.Minute)),
			}
		}).Read(t.Context())
		require.Zero(t, got)
		require.ErrorIs(t, err, errNegative)
		require.NotErrorIs(t, err, config.ErrNotFound)
		require.EqualError(t, err, "config READ_TIMEOUT: must not be negative\n"+
			"config WRITE_TIMEOUT: must not be negative")

		joined, ok := err.(interface{ Unwrap() []error })
		require.True(t, ok, "the error is an errors.Join of every failure")

		var names []string
		for _, e := range joined.Unwrap() {
			cerr, ok := errors.AsType[*config.Error](e)
			require.True(t, ok, "each failure is a *config.Error")
			require.ErrorIs(t, cerr, errNegative)
			names = append(names, cerr.Name)
		}
		require.Equal(t, []string{"READ_TIMEOUT", "WRITE_TIMEOUT"}, names)
	})

	t.Run("panics on a nil build function", func(t *testing.T) {
		t.Parallel()

		require.PanicsWithValue(t, "config: Struct requires a non-nil function", func() {
			config.Struct[int](nil)
		})
	})
}

func TestResolver_Get(t *testing.T) {
	t.Parallel()

	t.Run("panics on a nil Source", func(t *testing.T) {
		t.Parallel()

		v := config.Struct(func(r *config.Resolver) int {
			return r.Get[int](nil)
		})
		require.PanicsWithValue(t, "config: Get requires a non-nil Source", func() {
			_, _ = v.Read(t.Context())
		})
	})

	t.Run("panics when called after build returned", func(t *testing.T) {
		t.Parallel()

		var kept *config.Resolver
		_, err := config.Struct(func(r *config.Resolver) int {
			kept = r
			return 0
		}).Read(t.Context())
		require.NoError(t, err)
		require.NotNil(t, kept)

		require.PanicsWithValue(t, "config: Get called after the Struct build function returned", func() {
			kept.Get(config.Static(1))
		})
	})
}
