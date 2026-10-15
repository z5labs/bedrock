# Copilot Instructions for bedrock

## Project Overview

bedrock is a minimal foundation for building use case specific frameworks in Go. It is meant for teams, such as platform engineering teams, who build their own frameworks on top of it rather than for building services directly.

bedrock is being rebuilt around one idea: an application is an OS process. The standard streams, the environment variables, the interrupt signals and the exit code that the operating system gives a process are all handed to the application as a single value, and the application returns the status it exits with. The API is in active development and names will change, so these instructions deliberately do not describe it. To see the current API, run `go doc -all github.com/z5labs/bedrock`.

## Principles

1. **The process is the boundary.** Everything an application learns from, or does to, its OS process goes through the one value it is run with. Nothing else reaches the process.
2. **Explicit over implicit.** No global state, no hidden initialization and no magic. Every dependency is passed in.
3. **Small and composable.** Prefer a few small types that combine over a large type that tries to anticipate every use.
4. **Fail loudly.** Every error is checked. A broken invariant is a bug, so it panics, and the panic is never recovered.

## Build, Test, and Lint

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

See `.github/instructions/bedrock.instructions.md` for the rules that apply to Go files. They take precedence over `.github/instructions/go.instructions.md` where the two differ.
