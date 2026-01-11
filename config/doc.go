// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package config provides a functional approach to reading and composing configuration values.
//
// The package is built around the concept of a Reader[T], which represents a source of
// configuration values that may or may not be present. Readers can be composed using
// functional combinators to build complex configuration logic from simple building blocks.
//
// # Core Concepts
//
// Value[T] represents a configuration value that may or may not be set. This distinguishes
// between "not set" and "set to zero value", which is important for configuration with defaults.
//
// Reader[T] is an interface for reading configuration values. Readers are composable and
// can be chained together using combinators like Or, Map, Bind, and Default.
//
// # Basic Usage
//
// Read a value from an environment variable with a default:
//
//	port, err := config.Read(ctx,
//	    config.Default(8080, config.IntFromString(config.Env("PORT"))),
//	)
//
// Try multiple sources in order:
//
//	apiKey, err := config.Read(ctx,
//	    config.Or(
//	        config.Env("API_KEY"),
//	        config.Env("LEGACY_API_KEY"),
//	    ),
//	)
//
// Transform values using Map:
//
//	timeout := config.Map(
//	    config.Env("TIMEOUT"),
//	    func(ctx context.Context, s string) (time.Duration, error) {
//	        return time.ParseDuration(s)
//	    },
//	)
//
// # Composition
//
// Readers can be composed to build complex configuration logic:
//
//	dbConfig := config.Bind(
//	    config.Env("DB_CONFIG_FILE"),
//	    func(ctx context.Context, path string) config.Reader[DBConfig] {
//	        return config.Map(
//	            config.ReadFile(path),
//	            parseDBConfig,
//	        )
//	    },
//	)
//
// # Error Handling
//
// Readers distinguish between three states:
//   - Value is set (returns Value with set=true)
//   - Value is not set (returns Value with set=false, no error)
//   - Error occurred (returns error)
//
// The Read function converts "not set" to ErrValueNotSet for convenience.
package config
