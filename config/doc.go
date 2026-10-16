// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package config reads configuration values that are composed by chaining.
//
// A [Value] describes how to read one configuration value: where it comes
// from, how it is parsed, what it falls back to. Nothing is read until
// [Value.Read] is called. Inside [bedrock.App.Run], a value is read from the
// environment of the process the App was given:
//
//	port, err := config.Env(p.Environment, "PORT").Parse(strconv.Atoi).Default(8080).Read(ctx)
//
// The package never reads the environment of the current process itself, so
// an App that reads its configuration this way is tested whole, in
// parallel, with a [bedrock.StaticEnvironment] or bedrocktest.
//
// # Sources and values
//
// [Source] is the extension point: anything with a
// Read(context.Context) (T, error) method. It is satisfied structurally, so
// a package that provides a source does not import config. [From] wraps a
// Source in a Value, which gives it a name and the combinators.
//
// Value is the concrete type the combinators live on. [Value.Map],
// [Value.Parse] and [Value.FlatMap] are generic methods, which a Value has
// and an interface could not declare. [Value.Validate] checks a value with a
// func(T) error and returns it unchanged. [Env], [Func], [From] and [Static]
// construct one, and the zero Value is a value that is never set.
//
// # Checking a value
//
// [Value.Parse] turns a value into another; [Value.Validate] checks one
// without changing it. A check fails the value under its own name, and
// chained after [Value.Default] it checks the default too:
//
//	config.Env(p.Environment, "READ_TIMEOUT").Parse(time.ParseDuration).Default(5 * time.Second).Validate(nonNegative)
//
// fails with "config READ_TIMEOUT: must not be negative" whether the
// negative duration was set or was the default. A value that is not set is
// never checked, so a Default chained after Validate still applies.
//
// # Not found is an error
//
// A value that is not set is an error that satisfies
// errors.Is(err, [ErrNotFound]). [Value.Or] and [Value.Default] recover
// from that error and from no other, so a value that is present but
// malformed is never replaced by a fallback: with PORT=abc,
//
//	config.Env(p.Environment, "PORT").Parse(strconv.Atoi).Default(8080)
//
// fails rather than reading 8080. A variable set to the empty string is
// set, as it is for os.LookupEnv, so PORT= is a parse error too.
//
// Every error is wrapped once in an [*Error] naming the value that failed,
// and renders as "config PORT: <cause>"; a missing value renders as
// "config PORT: not found".
//
// # Building a struct
//
// [Struct] builds a configuration struct from many values, reading each one
// through [Resolver.Get]:
//
//	cfg := config.Struct(func(r *config.Resolver) Settings {
//		env := func(name string) config.Value[string] { return config.Env(p.Environment, name) }
//		return Settings{
//			Addr:        r.Get(env("ADDR")),
//			ReadTimeout: r.Get(env("READ_TIMEOUT").Parse(time.ParseDuration).Default(5 * time.Second)),
//		}
//	})
//
// Every Get is attempted, and every failure is reported together in one
// error, joined with errors.Join:
//
//	config ADDR: not found
//	config READ_TIMEOUT: time: missing unit in duration "30"
//
// A failed Get returns the zero value, so build must not use the result of
// one Get to decide what to read next. A read that depends on another
// value's result belongs in [Value.FlatMap], whose Value is passed to Get.
//
// # Migrating from v0.21.0
//
// MIGRATING.md, at the root of the bedrock repository, maps every export of
// config at v0.21.0 onto this package, with a worked example of moving an
// option from a Reader to a Value.
package config
