# Copilot Instructions for bedrock

## Project Overview

bedrock is a minimal, modular, and composable foundation for building use-case specific frameworks in Go. It's designed for platform engineering teams to build custom frameworks on top of it, not for building services directly.

## Build, Test, and Lint

```bash
# Build
go build ./...

# Run all tests with race detection and coverage
go test -race -cover ./...

# Run tests in a specific package
go test -race -cover ./config

# Run a single test
go test -race -v -run TestName ./path/to/package

# Lint (uses golangci-lint, config in .golangci.yaml)
golangci-lint run
```

The project maintains high test coverage, enforced in CI.

## Core Architecture

### Three Fundamental Abstractions

1. **Builder[T]** - Generic interface for constructing components with context support
2. **Runtime** - Interface representing a runnable application component (`Run(context.Context) error`)
3. **Runner[T]** - Interface for executing components built from Builders

### Functional Composition Pattern

Builders, Runners, and config Readers all support functional combinators:
- `Map[A, B](builder, mapper)` - Transform outputs
- `Bind[A, B](builder, binder)` - Chain together

### Config Three-State Values

The `config` package distinguishes between:
- Value is set (`Value[T]` with `set=true`)
- Value is not set (`Value[T]` with `set=false`, no error)
- Error occurred (returns error)

This is critical for proper default value handling with `config.Default()`.

## Key Conventions

### Function Types as Interface Implementations

The codebase extensively uses function types that implement interfaces:
- `BuilderFunc[T]` implements `Builder[T]`
- `RuntimeFunc` implements `Runtime`
- `RunnerFunc[T]` implements `Runner[T]`
- `ReaderFunc[T]` implements `Reader[T]` (in config package)

Use these when a simple function suffices instead of defining structs.

### Runner Wrapping Pattern

Compose runners by wrapping with cross-cutting concerns:
```go
runner := bedrock.NotifyOnSignal(
    bedrock.RecoverPanics(
        bedrock.DefaultRunner[bedrock.Runtime](),
    ),
    os.Interrupt, syscall.SIGTERM,
)
```

### Config Reader Composition

Chain config readers for fallback/transformation:
```go
logLevelReader := config.Default(
    "INFO",
    config.Env("MIN_LOG_LEVEL"),
)
```

## Package Structure

- `/` (root) - Core types: `Builder`, `Runtime`, `Runner`, and combinators
- `/config` - Configuration reading with functional composition
- `/runtime/http` - HTTP runtime with REST API support
- `/runtime/otel` - OpenTelemetry integration

## Design Principles

1. **Composability First** - Small components combined in various ways
2. **Functional Programming** - Generics, higher-order functions (Map, Bind), function types
3. **Minimal Opinions** - Abstracts "what" not "how"; users control initialization
4. **Type Safety** - Extensive use of Go generics
5. **Explicit Over Implicit** - No magic, no hidden global state
