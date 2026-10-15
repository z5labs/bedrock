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
// and an interface could not declare. [Env], [Func], [From] and [Static]
// construct one, and the zero Value is a value that is never set.
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
// # Migrating from v0.21.0
//
// Every export of config at v0.21.0 maps onto the package as follows.
//
//	v0.21.0                                process model
//	-------------------------------------  -----------------------------------------------------------
//	Reader[T]                              Source[T] (interface), Value[T] (concrete)
//	Value[T], ValueOf, Value.Value         removed: "not set" is ErrNotFound
//	ErrValueNotSet                         ErrNotFound
//	ReaderFunc[T], ReaderFunc.Read         Func(name, fn); From(name, src) wraps any Source[T]
//	ReaderOf(v)                            Static(v)
//	EmptyReader[T]()                       zero Value[T]{}, which reads as ErrNotFound
//	Map(r, func(ctx, A) (B, error))        v.Map(func(A) B) / v.Parse(func(A) (B, error))
//	Bind(r, func(ctx, A) Reader[B])        v.FlatMap(func(A) Value[B])
//	Or(r1, r2, ...)                        v1.Or(v2).Or(v3)
//	Default(d, r)                          v.Default(d)
//	Read(ctx, r)                           v.Read(ctx)
//	Must(ctx, r)                           removed: a config failure is an input error to return
//	MustOr(ctx, d, r)                      removed: v.Default(d), which recovers only from ErrNotFound
//	Env(name)                              Env(p.Environment, name)
//	ReadFile(path)                         removed until a consumer exists; a future File would take an fs.FS
//	BoolFromString                         .Parse(strconv.ParseBool)
//	IntFromString                          .Parse(strconv.Atoi)
//	DurationFromString                     .Parse(time.ParseDuration)
//	Int64FromString                        .Parse(ParseInt64)
//	Float64FromString                      .Parse(ParseFloat64)
//	Int64FromBytes(order, r)               removed
//	—                                      new: OneOf(...), *Error{Name, Err}
package config
