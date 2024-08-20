# bedrock
[![Mentioned in Awesome Go](https://awesome.re/mentioned-badge.svg)](https://github.com/avelino/awesome-go)
[![Go Reference](https://pkg.go.dev/badge/github.com/z5labs/bedrock.svg)](https://pkg.go.dev/github.com/z5labs/bedrock)
[![Go Report Card](https://goreportcard.com/badge/github.com/z5labs/bedrock)](https://goreportcard.com/report/github.com/z5labs/bedrock)
![Coverage](https://img.shields.io/badge/Coverage-91.5%25-brightgreen)
[![build](https://github.com/z5labs/bedrock/actions/workflows/build.yaml/badge.svg)](https://github.com/z5labs/bedrock/actions/workflows/build.yaml)

**bedrock provides a minimal, modular and composable foundation for
quickly developing services and more use case specific frameworks in Go.**

# Core Concepts

`bedrock` begins with the concepts of an `App` and a `Runtime`. `App`
is a container for a `Runtime` and handles more "low-level" things,
such as, OS interrupts, config parsing, environment variable overrides, etc.
The `Runtime` is then the users entry point for jumping right into
their use case specific code e.g. RESTful API, gRPC service, K8s job, etc.

# Building services with bedrock

`bedrock` conveniently comes with a couple of `Runtime`s already implemented for you.
This can significantly aid in shortening your overall development time, as well as,
provide an example for how to implement your own custom `Runtime`.

For example, the below shows how simple it is to initialize an HTTP based "API" leveraging
the builtin HTTP Runtime.
```go
package main

import (
    "context"
    "net/http"

    "github.com/z5labs/bedrock"
    "github.com/z5labs/bedrock/http"
)

func initRuntime(ctx context.Context) (bedrock.Runtime, error) {
    rt := brhttp.NewRuntime(
		brhttp.ListenOnPort(8080),
		brhttp.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "Hello, world")
		}),
	)
	return rt, nil
}

func main() {
	bedrock.New(
		bedrock.WithRuntimeBuilderFunc(initRuntime),
	).Run()
}
```

There you go! An entire HTTP API in less than 20 lines... well kind of. This incredibly
simple example can easily be extended (aka made more production-ready) by supplying extra
options to both `bedrock.New` and `brhttp.NewRuntime`. For available, options please
check the official Go documentation.

# Building custom frameworks with bedrock

One of the guiding principals for `bedrock` is to composable. This principal comes
from the experience gained from working with custom, tailor made frameworks which
over their lifetime within an organization are unable to adapt to changing
development and deployment patterns. Eventually, these frameworks are abandoned
for new ones or completely rewritten to reflect the current state of the organization.

`bedrock` defines a small set of types and carefully chooses its opinions to balance
composability and functionality, as much as it can. The result is, in fact, a framework
that isn't necessarily designed for building services directly, but instead meant for building
more custom, use case specific frameworks.

For example, `bedrock` could be used by your organizations platform engineering or framework
team(s) to quickly develop internal frameworks which abstract over all of your organizations
requirements e.g. OpenTelemetry, Logging, Authenticated endpoints, etc. Then, due to the high composibility
of `bedrock`, any changes within your organization would then be very easy to adapt to within
your internal framework. A more concrete example of how a custom framework could look like
can be found in [example/custom_framework](https://github.com/z5labs/bedrock/tree/main/example/custom_framework).