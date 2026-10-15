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

golangci-lint's forbidigo enforces this module-wide. Only `os.go`, `os_test.go` and `bedrocktest/conformance_unix_test.go`, which checks the fake signal delivery against the real one, are excluded.

## Parsing flags

- Never use `flag.Parse`, `flag.CommandLine` or `flag.ExitOnError`. Parse `p.Args[1:]` with a `flag.FlagSet` of your own, using `flag.ContinueOnError` and `p.Stderr` as its output.
- `flag` itself prints the parse error and the usage, and `Main` prints nothing, so do not print the error again. Return it as the cause, which is for tests. `-h` is a successful exit.

Use this idiom, which `ExampleApp_flags` runs whole:

```go
fs := flag.NewFlagSet(p.Args[0], flag.ContinueOnError)
fs.SetOutput(p.Stderr) // flag reports the parse error and usage to the user
n := fs.Int("n", 1, "how many times to greet")
switch err := fs.Parse(p.Args[1:]); {
case errors.Is(err, flag.ErrHelp):
	return bedrock.Exit(0)
case err != nil:
	return bedrock.ExitWithCause(2, err) // cause is for tests; flag already told the user
}
```

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
- Library packages keep 100% statement coverage. CI fails a pull request that leaves any non-main package below 100.0%.
