# app

This a modular and composable framework for quickly developing services in Go.

# Runtime

At the core of the app module is the Runtime interface. The top-level
app package consumes this interface and wraps in a CLI implementation
which handles low level things like signal interrupts. This brings the
development bar up from CLI to the "Runtime" level which helps remove
some cognitive load for developers.

The Runtime interface also allows the top-level package to support running
multiple "apps" at once. An example use case for this would be, writing a
gRPC service but for its health checks you implement HTTP based endpoints.

## gRPC

Provides a runtime mplementation for gRPC.

## HTTP

Provides a runtime implementation for HTTP.

## Queue

Provides a runtime implementation for applications which consume events from a queue.