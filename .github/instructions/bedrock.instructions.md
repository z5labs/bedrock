---
description: 'bedrock-specific rules for Go code that take precedence over the generic Go instructions'
applyTo: '**/*.go'
---

# bedrock Go Instructions

These rules are specific to bedrock. **Where they differ from `go.instructions.md`, these rules take precedence.**

bedrock models an application as an OS process. The application is handed everything it gets from the process as the single value passed to `Run`, and it returns the status it exits with.

## The OS edge

Exactly one file in the repository is the OS edge: the file that builds the value passed to `Run` from the real OS process, and that turns the returned status into the process exit code with `os.Exit`.

- Do not use `os.Args`, `os.Getenv`, `os.LookupEnv`, `os.Environ`, `os.Stdin`, `os.Stdout`, `os.Stderr`, `os.Exit` or any `signal.Notify*` function (`signal.Notify`, `signal.NotifyContext`) anywhere except that single OS-edge file. This applies to tests and examples as well as library code.
- Everywhere else, read arguments, environment variables and standard streams, and listen for signals, only through the value passed to `Run`.

## The value passed to `Run`

- Every field of the value passed to `Run` is required. This overrides "Make the zero value useful" in `go.instructions.md`: code that receives this value may use any field without a nil check, and code that constructs it must set every field.
- A missing field is a programming error, not a condition to handle gracefully.

## Panics

- Panics are assertions. Panic only when an invariant is broken by a programming error.
- Do not recover panics. Never call `recover`, and never wrap an application to turn a panic into an error or an exit status.

## Errors and tests

- Check every error. Return it, wrap it with context, or handle it.
- Library code imports only the standard library.
- Tests assert with `github.com/stretchr/testify/require`.
- Library packages keep 100% statement coverage.
