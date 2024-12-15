# bedrock
[![Mentioned in Awesome Go](https://awesome.re/mentioned-badge.svg)](https://github.com/avelino/awesome-go)
[![Go Reference](https://pkg.go.dev/badge/github.com/z5labs/bedrock.svg)](https://pkg.go.dev/github.com/z5labs/bedrock)
[![Go Report Card](https://goreportcard.com/badge/github.com/z5labs/bedrock)](https://goreportcard.com/report/github.com/z5labs/bedrock)
![Coverage](https://img.shields.io/badge/Coverage-98.6%25-brightgreen)
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

```go
type App interface {
	Run(context.Context) error
}
```

[App](https://pkg.go.dev/github.com/z5labs/bedrock#App) is a
simple abstraction over the execution of your specific application type
e.g. HTTP server, gRPC server, etc.

```go
type AppBuilder[T any] interface {
	Build(ctx context.Context, cfg T) (App, error)
}
```

[AppBuilder](https://pkg.go.dev/github.com/z5labs/bedrock#AppBuilder) puts
the responsibility of [App](https://pkg.go.dev/github.com/z5labs/bedrock#App) initialization
in your hands!

The generic parameter provided to your [AppBuilder](https://pkg.go.dev/github.com/z5labs/bedrock#AppBuilder)
is, in fact, your custom configuration type, which means no messing with config
parsing and unmarshalling yourself!

```go
package config

type Source interface {
	Apply(Store) error
}
```

The [config.Source](https://pkg.go.dev/github.com/z5labs/bedrock/pkg/config#Source) is
arguably the most powerful abstraction defined in any of the [bedrock](https://pkg.go.dev/github.com/z5labs/bedrock)
packages. It abstracts over the entire mechanic of sourcing your application configuration.
This simple interface can then be implemented in various ways to support loading configuration
from different files (e.g. YAML, JSON, TOML) to remote configuration stores (e.g. etcd).

```go
func Run[T any](ctx context.Context, builder AppBuilder[T], srcs ...config.Source) error
```

The final piece and most crucial piece [bedrock](https://pkg.go.dev/github.com/z5labs/bedrock)
provides is the [Run](https://pkg.go.dev/github.com/z5labs/bedrock#Run) function which
handles the orchestration of config parsing, app building and, lastly, app execution by relying
on the other core abstractions noted above.

## Putting them altogether

Below is a tiny and simplistic example of all the core concepts of [bedrock](https://pkg.go.dev/github.com/z5labs/bedrock).

### config.yaml

```yaml
logging:
	min_level: {{env "MIN_LOG_LEVEL"}}
```

### main.go

```go
package main

import (
	"bytes"
	"context"
	_ "embed"
	"log/slog"
	"os"

	"github.com/z5labs/bedrock"
	"github.com/z5labs/bedrock/app"
	"github.com/z5labs/bedrock/appbuilder"
	"github.com/z5labs/bedrock/config"
)

//go:embed config.yaml
var configBytes []byte

func main() {
	os.Exit(run())
}

func run() int {
	// bedrock does not handle process exiting for you. This is mostly
	// to aid framework developers in unit testing their usages of bedrock
	// by validating the returned error.
	err := bedrock.Run(
		context.Background(),
		appbuilder.Recover(
			bedrock.AppBuilderFunc[myConfig](initApp),
		),
		config.FromYaml(
			config.RenderTextTemplate(
				bytes.NewReader(configBytes),
				config.TemplateFunc("env", os.Getenv),
			),
		),
	)
	if err == nil {
		return 0
	}
	return 1
}

// myConfig can contain anything you like. The only thing you must
// remember is to always use the tag name, "config". If that tag
// name is not used then the bedrock config package will not know
// how to properly unmarshal the config source(s) into your custom
// config struct.
type myConfig struct {
	Logging struct {
		MinLevel slog.Level `config:"min_level"`
	} `config:"logging"`
}

type myApp struct {
	log *slog.Logger
}

// initApp is a function implementation of the bedrock.AppBuilder interface.
func initApp(ctx context.Context, cfg myConfig) (bedrock.App, error) {
	var base bedrock.App = &myApp{
		log: slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: cfg.Logging.MinLevel,
		})),
	}
	base = app.Recover(base)
	return base, nil
}

// Run implements the bedrock.App interface.
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