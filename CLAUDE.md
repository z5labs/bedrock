# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

bedrock is a minimal foundation for building use case specific frameworks in Go. It is meant for teams, such as platform engineering teams, who build their own frameworks on top of it rather than for building services directly.

bedrock is built around one idea: an application is an OS process. The arguments, the standard streams, the environment variables, the signals and the exit code that the operating system gives a process are all handed to the application as a single value, and the application returns the status it exits with. Framework features, such as an HTTP server, a REST DSL or OpenTelemetry setup, belong in z5labs/humus, which is built on bedrock.

## Principles

1. **The process is the boundary.** Everything an application learns from, or does to, its OS process goes through the one value it is run with. Nothing else reaches the process.
2. **Explicit over implicit.** No global state, no hidden initialization and no magic. Every dependency is passed in.
3. **Small and composable.** Prefer a few small types that combine over a large type that tries to anticipate every use.
4. **Fail loudly.** Every error is checked. A broken invariant is a bug, so it panics, and the panic is never recovered.

## API

An application is a `bedrock.App`:

```go
type App interface {
	Run(ctx context.Context, p bedrock.Process) bedrock.ExitStatus
}

func main() { bedrock.Main(app) }
```

- **`Process`** is what exec hands a process: `Args`, `Stdin`, `Stdout`, `Stderr`, `Environment` and `NotifyContext`, which delivers signals with the semantics of `signal.NotifyContext`. Every field is required.
- **`ExitStatus`** is opaque. `Exit(code)` and `ExitWithCause(code, err)` build one and panic on a code outside 0..255; the zero `ExitStatus` is success. `Code()` is the exit status of the process. `Cause()` is for tests and other callers of `Run`; `Main` ignores it.
- **`Main(app)`** is the only place bedrock touches the real process. It runs `app` on `OSProcess()` with `context.Background()`, exits with the code, and never prints the cause, so an `App` writes anything a user should see to `p.Stderr` itself.
- **`Environment`** offers lookup only: `Value(name)` returns a `ValueNotFoundError` for a variable that is not set. `OSEnvironment` reads the real process and `StaticEnvironment` holds fixed values for tests.
- **`AppFunc`** adapts a function to `App`.
- **`bedrocktest.NewProcess(t, opts...)`** builds a hermetic fake `Process` with `WithArgs`, `WithEnv` and `WithStdin`. A test drives its signals with `Notified` and `Signal`, and reads its output with `Stdout` and `Stderr`.
- **`config.Value[T]`** is a configuration value composed by chaining, such as `config.Env(p.Environment, "PORT").Parse(strconv.Atoi).Default(8080)`. "Not set" is `config.ErrNotFound`, and `Or` and `Default` recover from it and from nothing else. `config.Struct` reads many values and reports every failure in one error.

Run `go doc -all` on a package for the full API. `MIGRATING.md` maps every export of v0.21.0 onto this API, onto z5labs/humus, or onto the standard library.

## Package Layout

```
.                    package bedrock: App, Process, ExitStatus, Environment, Main
  os.go              the OS edge: OSProcess, OSEnvironment and Main, the only file that touches the real process
bedrocktest/         a hermetic fake Process for tests that run a whole App
config/              chainable configuration values and config.Struct
example/httpserver/  the reference service: net/http alone, configuration, graceful shutdown, synctest tests
MIGRATING.md         where every v0.21.0 export went
```

## Development Commands

```bash
# Build
go build ./...

# Vet
go vet ./...

# Run all tests with race detection and coverage
go test -race -cover ./...

# Run a single test or example
go test -race -run Example ./...

# Per-function coverage report
go test ./... -covermode=count -coverprofile=coverage.out
go tool cover -func=coverage.out

# Lint (config in .golangci.yaml; requires golangci-lint built with go1.27 or newer)
golangci-lint run ./...
```

## Conventions

- **Library code imports only the standard library.** Third-party modules are allowed in tests only.
- **Tests assert with `github.com/stretchr/testify/require`.**
- **The process is reached only through the value passed to `Run`.** Library and application code never read the environment, the standard streams or the signals of the current process directly, and never exit the process themselves. A single file at the OS edge builds that value from the real process and turns the returned status into the process exit code; it is the only place allowed to do so.
- **Panics are assertions.** Panic only on a programming error, such as a broken invariant or a missing required field, and never recover a panic.
- **Every error is checked.** Return it, wrap it with context, or handle it. Never discard one silently.
- **Library packages have 100% statement coverage.**

The Copilot instructions in `.github/instructions/bedrock.instructions.md` state the same rules for Go files, and take precedence over the generic `.github/instructions/go.instructions.md` where the two differ.
