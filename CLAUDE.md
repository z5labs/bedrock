# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**bedrock** is a minimal, modular, and composable foundation for building use-case specific frameworks in Go. It's not designed for building services directly, but rather for building more custom frameworks on top of it (e.g., for platform engineering teams).

The codebase recently underwent a major refactor (issue-447) to adopt a more functional programming approach using generic builders, runtimes, and runners.

## Core Architecture

bedrock is built around three fundamental abstractions:

### 1. Builder[T]
A generic interface for constructing application components with context support:
```go
type Builder[T any] interface {
    Build(context.Context) (T, error)
}
```

Builders can be composed using functional combinators:
- `Map[A, B](builder Builder[A], mapper func(A) (B, error))` - Transform builder outputs
- `Bind[A, B](builder Builder[A], binder func(A) Builder[B])` - Chain builders together

### 2. Runtime
An interface representing a runnable application component:
```go
type Runtime interface {
    Run(context.Context) error
}
```

### 3. Runner[T]
An interface for executing application components built from Builders:
```go
type Runner[T Runtime] interface {
    Run(context.Context, Builder[T]) error
}
```

Available runner wrappers:
- `NotifyOnSignal` - Listen for OS signals and cancel context
- `RecoverPanics` - Recover from panics and return as errors
- `DefaultRunner` - Build and run the application component

### Configuration System (config package)

The `config` package provides a functional approach to reading configuration:

- `Reader[T]` - Interface for reading configuration values that may or may not be present
- `Value[T]` - Represents a value that distinguishes between "not set" and "set to zero value"

Configuration readers can be composed:
- `Or(readers...)` - Try multiple sources in order
- `Map(reader, mapper)` - Transform values
- `Bind(reader, binder)` - Chain readers together
- `Default(defaultVal, reader)` - Provide defaults
- `Env(name)` - Read from environment variables

## Development Commands

### Build
```bash
go build ./...
```

### Run Tests
```bash
# Run all tests with race detection and coverage
go test -race -cover ./...

# Run tests in a specific package
go test -race -cover ./config

# Generate coverage report (used in CI)
GOEXPERIMENT=nocoverageredesign go test -v ./... -covermode=count -coverprofile=coverage.out
go tool cover -func=coverage.out -o=coverage.out
```

### Lint
```bash
# Uses golangci-lint (config in .golangci.yaml)
golangci-lint run
```

The project maintains a very high test coverage (97.5%+), which is enforced in CI.

## Code Structure

The repository has a flat, minimal structure:

- `/` (root) - Core bedrock types and interfaces (Builder, Runtime, Runner)
- `/config` - Configuration reading system with functional composition
- Tests use `_test.go` suffix, examples use `_example_test.go` suffix

All major abstractions are defined at the package level with minimal nesting, emphasizing composability over deep hierarchies.

## Design Principles

1. **Composability First** - Small, reusable components that can be combined in various ways
2. **Functional Programming** - Heavy use of generics, higher-order functions (Map, Bind), and function types
3. **Minimal Opinions** - Abstracts the "what" not the "how" - users control initialization and execution
4. **Type Safety** - Uses Go generics extensively for type-safe configuration and component building
5. **Explicit Over Implicit** - No magic, no hidden global state, clear error handling

## Key Patterns

### Function Types as Interface Implementations
The codebase extensively uses function types that implement interfaces:
- `BuilderFunc[T]` implements `Builder[T]`
- `RuntimeFunc` implements `Runtime`
- `RunnerFunc[T]` implements `Runner[T]`
- `ReaderFunc[T]` implements `Reader[T]` (in config package)

This allows using simple functions where interfaces are expected without defining structs.

### Three-State Values in Config
The config package distinguishes between:
- Value is set (returns `Value[T]` with `set=true`)
- Value is not set (returns `Value[T]` with `set=false`, no error)
- Error occurred (returns error)

This is critical for proper default value handling.

## Recent Changes

The codebase underwent a major refactor (commits 858ea1f - ef59864) that:
- Removed the old lifecycle package
- Introduced the new functional Builder/Runtime/Runner architecture
- Refactored config to use functional Reader pattern
- Uses Go 1.24+ for latest generics features
