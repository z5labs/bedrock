# bedrock

bedrock provides a minimal, modular and composable foundation for
quickly developing services and more use case specific frameworks in Go.

# Runtime

At the core of the bedrock module is the Runtime interface. The top-level
bedrock package consumes this interface and wraps in a CLI implementation
which handles low level things like signal interrupts. This brings the
development bar up from CLI to the "Runtime" level which helps remove
some cognitive load for developers.

The Runtime interface also allows the top-level package to support running
multiple runtimes at once. An example use case for this would be, writing a
gRPC service but for its health checks you implement HTTP based endpoints
instead of doing health checks via gRPC.

## gRPC

Provides a runtime mplementation for gRPC.

## HTTP

Provides a runtime implementation for HTTP.

## Queue

Provides a runtime implementation for services which consume events from a queue.