# bedrock
[![Mentioned in Awesome Go](https://awesome.re/mentioned-badge.svg)](https://github.com/avelino/awesome-go)
[![Go Reference](https://pkg.go.dev/badge/github.com/z5labs/bedrock.svg)](https://pkg.go.dev/github.com/z5labs/bedrock)
[![Go Report Card](https://goreportcard.com/badge/github.com/z5labs/bedrock)](https://goreportcard.com/report/github.com/z5labs/bedrock)
![Coverage](https://img.shields.io/badge/Coverage-88.0%25-brightgreen)
[![build](https://github.com/z5labs/bedrock/actions/workflows/build.yaml/badge.svg)](https://github.com/z5labs/bedrock/actions/workflows/build.yaml)

**bedrock provides a minimal, modular and composable foundation for
quickly developing more use case specific frameworks in Go.**

# Building custom frameworks with bedrock

One of the guiding principals for [bedrock](https://pkg.go.dev/github.com/z5labs/bedrock) is to be composable.
This principal comes from the experience gained from working with custom, tailor made frameworks which
over their lifetime within an organization are unable to adapt to changing
development and deployment patterns. Eventually, these frameworks are abandoned
for new ones or completely rewritten to reflect the current state of the organization.

[bedrock](https://pkg.go.dev/github.com/z5labs/bedrock) defines a small set of types and carefully
chooses its opinions to balance composability and functionality, as much as it can. The result is, in fact, a framework
that isn't necessarily designed for building services directly, but instead meant for building
more custom, use case specific frameworks.

For example, [bedrock](https://pkg.go.dev/github.com/z5labs/bedrock) could be used by your organizations
platform engineering or framework team(s) to quickly develop internal frameworks which abstract over all of
your organizations requirements e.g. OpenTelemetry, Logging, Authenticated endpoints, etc. Then, due to the
high composibility of [bedrock](https://pkg.go.dev/github.com/z5labs/bedrock), any changes within your
organization would then be very easy to adapt to within your internal framework.

## Core Concepts

### Builder

```go
type Builder[T any] interface {
	Build(context.Context) (T, error)
}
```

[Builder](https://pkg.go.dev/github.com/z5labs/bedrock#Builder) is a
generic interface for constructing application components with context support.
Builders can be composed using functional combinators like
[Map](https://pkg.go.dev/github.com/z5labs/bedrock#Map) and
[Bind](https://pkg.go.dev/github.com/z5labs/bedrock#Bind).

### Runtime

```go
type Runtime interface {
	Run(context.Context) error
}
```

[Runtime](https://pkg.go.dev/github.com/z5labs/bedrock#Runtime) is a
simple abstraction over the execution of your specific application type
e.g. HTTP server, gRPC server, background worker, etc.

### Runner

```go
type Runner[T Runtime] interface {
	Run(context.Context, Builder[T]) error
}
```

[Runner](https://pkg.go.dev/github.com/z5labs/bedrock#Runner) executes
application components built from Builders. Runners can be wrapped to add
cross-cutting concerns like signal handling with
[NotifyOnSignal](https://pkg.go.dev/github.com/z5labs/bedrock#NotifyOnSignal)
and panic recovery with
[RecoverPanics](https://pkg.go.dev/github.com/z5labs/bedrock#RecoverPanics).

### Configuration

```go
package config

type Reader[T any] interface {
	Read(context.Context) (Value[T], error)
}
```

The [config.Reader](https://pkg.go.dev/github.com/z5labs/bedrock/config#Reader) is
arguably the most powerful abstraction defined in any of the [bedrock](https://pkg.go.dev/github.com/z5labs/bedrock)
packages. It abstracts over reading configuration values that may or may not be present,
distinguishing between "not set" and "set to zero value." Readers can be composed using
functional combinators like
[Or](https://pkg.go.dev/github.com/z5labs/bedrock/config#Or),
[Map](https://pkg.go.dev/github.com/z5labs/bedrock/config#Map),
[Bind](https://pkg.go.dev/github.com/z5labs/bedrock/config#Bind), and
[Default](https://pkg.go.dev/github.com/z5labs/bedrock/config#Default).

## Putting them altogether

Below is a tiny and simplistic example of all the core concepts of [bedrock](https://pkg.go.dev/github.com/z5labs/bedrock).

### main.go

```go
package main

import (
	"context"
	"log/slog"
	"os"
	"syscall"

	"github.com/z5labs/bedrock"
	"github.com/z5labs/bedrock/config"
)

func main() {
	os.Exit(run())
}

func run() int {
	// Create a runner with signal handling and panic recovery
	runner := bedrock.NotifyOnSignal(
		bedrock.RecoverPanics(
			bedrock.DefaultRunner[bedrock.Runtime](),
		),
		os.Interrupt,
		os.Kill,
		syscall.SIGTERM,
	)

	// Build and run the application
	err := runner.Run(
		context.Background(),
		bedrock.BuilderFunc[bedrock.Runtime](buildApp),
	)
	if err == nil {
		return 0
	}
	return 1
}

type myApp struct {
	log *slog.Logger
}

// buildApp constructs the application using functional config composition
func buildApp(ctx context.Context) (bedrock.Runtime, error) {
	// Read log level from environment with a default
	logLevelReader := config.Default(
		"INFO",
		config.Env("MIN_LOG_LEVEL"),
	)

	logLevel, err := config.Read(ctx, logLevelReader)
	if err != nil {
		return nil, err
	}

	// Parse the log level string
	var level slog.Level
	err = level.UnmarshalText([]byte(logLevel))
	if err != nil {
		return nil, err
	}

	return &myApp{
		log: slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: level,
		})),
	}, nil
}

// Run implements the bedrock.Runtime interface.
func (a *myApp) Run(ctx context.Context) error {
	// Do something here like:
	// - run an HTTP server
	// - start the AWS lambda runtime,
	// - run goroutines to consume from Kafka
	//   etc.

	a.log.InfoContext(ctx, "running my app")
	return nil
}
```

# Built with bedrock

- [z5labs/humus](https://github.com/z5labs/humus)