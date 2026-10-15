# bedrock
[![Mentioned in Awesome Go](https://awesome.re/mentioned-badge.svg)](https://github.com/avelino/awesome-go)
[![Go Reference](https://pkg.go.dev/badge/github.com/z5labs/bedrock.svg)](https://pkg.go.dev/github.com/z5labs/bedrock)
[![Go Report Card](https://goreportcard.com/badge/github.com/z5labs/bedrock)](https://goreportcard.com/report/github.com/z5labs/bedrock)
[![build](https://github.com/z5labs/bedrock/actions/workflows/build.yaml/badge.svg)](https://github.com/z5labs/bedrock/actions/workflows/build.yaml)

**bedrock is a minimal foundation for building use case specific frameworks in Go.**

bedrock starts from one idea: an application is an OS process. Everything a
process is handed by the operating system (its standard streams, its
environment, its signals and its exit code) crosses a single, explicit boundary
into the application, and nothing reaches around that boundary to touch the
process directly. Keeping the boundary narrow and explicit is what lets the
frameworks built on bedrock stay small, composable and testable without a real
process behind them.

The API is in active development and is not yet stable. If you are using the
previous Builder/Runtime/Runner API, its documentation remains available at
[pkg.go.dev/github.com/z5labs/bedrock@v0.21.0](https://pkg.go.dev/github.com/z5labs/bedrock@v0.21.0).
