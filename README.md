# bedrock
[![Mentioned in Awesome Go](https://awesome.re/mentioned-badge.svg)](https://github.com/avelino/awesome-go)
[![Go Reference](https://pkg.go.dev/badge/github.com/z5labs/bedrock.svg)](https://pkg.go.dev/github.com/z5labs/bedrock)
[![Go Report Card](https://goreportcard.com/badge/github.com/z5labs/bedrock)](https://goreportcard.com/report/github.com/z5labs/bedrock)
![Coverage](https://img.shields.io/badge/Coverage-97.0%25-brightgreen)
[![build](https://github.com/z5labs/bedrock/actions/workflows/build.yaml/badge.svg)](https://github.com/z5labs/bedrock/actions/workflows/build.yaml)

**bedrock provides a minimal, modular and composable foundation for
quickly developing services and more use case specific frameworks in Go.**

# Core Concepts

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

# Building services with bedrock

[bedrock](https://pkg.go.dev/github.com/z5labs/bedrock) conveniently comes with a couple of
[App](https://pkg.go.dev/github.com/z5labs/bedrock#App)s already implemented for you.
This can significantly aid in shortening your overall development time, as well as,
provide an example for how to implement your own custom [App](https://pkg.go.dev/github.com/z5labs/bedrock#App).

For example, the below shows how simple it is to implement a RESTful API leveraging
the [rest.App](https://pkg.go.dev/github.com/z5labs/bedrock/rest#App).

```go
package main

import (
    "context"
	"encoding/json"
	"net/http"
	"strings"

    "github.com/z5labs/bedrock"
    "github.com/z5labs/bedrock/pkg/config"
    "github.com/z5labs/bedrock/rest"
	"github.com/z5labs/bedrock/rest/endpoint"
)

type echoService struct{}

type EchoRequest struct {
	Msg string `json:"msg"`
}

func (EchoRequest) ContentType() string {
	return "application/json"
}

func (req *EchoRequest) UnmarshalBinary(b []byte) error {
	return json.Unmarshal(b, req)
}

type EchoResponse struct {
	Msg string `json:"msg"`
}

func (EchoResponse) ContentType() string {
	return "application/json"
}

func (resp EchoResponse) MarshalBinary() ([]byte, error) {
	return json.Marshal(resp)
}

func (echoService) Handle(ctx context.Context, req *EchoRequest) (*EchoResponse, error) {
	return &EchoResponse{Msg: req.Msg}, nil
}

type Config struct {
	Title string `config:"title"`
	Version string `config:"version"`

	Http struct {
		Port uint `config:"port"`
	} `config:"http"`
}

// here we're defining our AppBuilder as a simple function
// remember bedrock.Run handles config unmarshalling for us
// so we get to work with our custom config type, Config, directly.
func buildRestApp(ctx context.Context, cfg Config) (bedrock.App, error) {
	app := rest.NewApp(
		rest.ListenOn(cfg.Http.Port),
		rest.Title(cfg.Title),
		rest.Version(cfg.Version),
		rest.Endpoint(
			http.MethodPost,
			"/",
			endpoint.NewOperation(
				echoService{},
			),
		),
	)
	return app, nil
}

// would recommend this being a separate file you could use
// go:embed on or use a config.Source that could fetch it
// from a remote store.
var config = `{
	"title": "My Example API",
	"version": "v0.0.0",
	"http": {
		"port": 8080
	}
}`

func main() {
	builder := bedrock.AppBuilderFunc[Config](buildRestApp)

	// Note: Should actually handle error in your code
	_ = bedrock.Run(
		context.Background(),
		builder,
		config.FromJson(strings.NewReader(config)),
	)
}
```

There you go, an entire RESTful API in less than 100 lines!

This incredibly simple example can theb easily be extended (aka made more production-ready) by leveraging a middleware
approach to the [App](https://pkg.go.dev/github.com/z5labs/bedrock#App) returned by your
[AppBuilder](https://pkg.go.dev/github.com/z5labs/bedrock#AppBuilder). Conventiently,
[bedrock](https://pkg.go.dev/github.com/z5labs/bedrock) already has a couple common middlewares
defined for your use in [package app](https://pkg.go.dev/github.com/z5labs/bedrock/pkg/app). Two
notable middleware implementations are:

- [WithOTel](https://pkg.go.dev/github.com/z5labs/bedrock/pkg/app#WithOTel) initializes
the various global [OpenTelemetry](https://opentelemetry.io/) types (e.g. TracerProvider, MeterProvider, etc.)
before executing your actual [App](https://pkg.go.dev/github.com/z5labs/bedrock#App).
- [WithSignalNotifications](https://pkg.go.dev/github.com/z5labs/bedrock/pkg/app#WithSignalNotifications)
wraps your [App](https://pkg.go.dev/github.com/z5labs/bedrock#App) and will execute it with a
"child" [context.Context](https://pkg.go.dev/context#Context) which will automatically be cancelled
if any of the provided [os.Signal](https://pkg.go.dev/os#Signal)s are received.

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
organization would then be very easy to adapt to within your internal framework. A more concrete example of
how a custom framework could look like can be found in [example/custom_framework](https://github.com/z5labs/bedrock/tree/main/example/custom_framework).