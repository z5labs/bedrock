# bedrock
[![Mentioned in Awesome Go](https://awesome.re/mentioned-badge.svg)](https://github.com/avelino/awesome-go)
[![Go Reference](https://pkg.go.dev/badge/github.com/z5labs/bedrock.svg)](https://pkg.go.dev/github.com/z5labs/bedrock)
[![Go Report Card](https://goreportcard.com/badge/github.com/z5labs/bedrock)](https://goreportcard.com/report/github.com/z5labs/bedrock)
[![build](https://github.com/z5labs/bedrock/actions/workflows/build.yaml/badge.svg)](https://github.com/z5labs/bedrock/actions/workflows/build.yaml)

**bedrock is a minimal foundation for building use case specific frameworks in Go.**

It is meant for teams, such as platform engineering teams, who build their own
frameworks on top of it. If you want a framework to build services with, see
[z5labs/humus](https://github.com/z5labs/humus), which is built on bedrock.

## An application is an OS process

The operating system hands a process its arguments, its environment, its
standard streams and the delivery of its signals, and takes back an exit
status. bedrock hands an application exactly those things, as one
`bedrock.Process` value, and takes back a `bedrock.ExitStatus`:

```go
type App interface {
	Run(ctx context.Context, p bedrock.Process) bedrock.ExitStatus
}
```

That call is the boundary. An `App` never reads `os.Args`, `os.Getenv` or
`os.Stdout`, never calls `signal.Notify` and never calls `os.Exit`. Everything
it learns from its process comes through `p`, and the only thing it does to its
process is return a status:

```go
var greeter = bedrock.AppFunc(func(ctx context.Context, p bedrock.Process) bedrock.ExitStatus {
	greeting, err := config.Env(p.Environment, "GREETING").Default("hello").Read(ctx)
	if err != nil {
		return fail(p, err)
	}

	ctx, stop := p.NotifyContext(ctx, syscall.SIGTERM)
	defer stop()

	if _, err := fmt.Fprintln(p.Stdout, greeting); err != nil {
		return fail(p, err)
	}
	<-ctx.Done()
	return bedrock.ExitWithCause(0, context.Cause(ctx))
})

// fail reports err on the App's standard error, because bedrock.Main prints
// nothing, and returns it as the cause of the status so a test can assert on it.
func fail(p bedrock.Process, err error) bedrock.ExitStatus {
	if _, werr := fmt.Fprintln(p.Stderr, err); werr != nil {
		return bedrock.ExitWithCause(1, errors.Join(err, werr))
	}
	return bedrock.ExitWithCause(1, err)
}
```

`main` is one line. `bedrock.Main` runs the `App` on the real process and exits
with the code of the status it returns:

```go
func main() { bedrock.Main(greeter) }
```

Because nothing reaches around the boundary, a test runs the whole `App` on a
fake process. It never touches the environment, the streams or the signals of
the test binary, so it runs in parallel and inside a `testing/synctest` bubble:

```go
func TestGreeter(t *testing.T) {
	t.Parallel()

	fp, statusCh := bedrocktest.Start(t, greeter, bedrocktest.WithEnv(map[string]string{"GREETING": "hi"}))

	<-fp.Notified(syscall.SIGTERM)
	fp.Signal(syscall.SIGTERM)
	status := <-statusCh

	require.Equal(t, 0, status.Code())
	require.Equal(t, "hi\n", fp.Stdout())
}
```

## Packages

| package | what it is |
| --- | --- |
| [bedrock](https://pkg.go.dev/github.com/z5labs/bedrock) | `App`, `Process`, `ExitStatus`, `Environment` and `Main`, the one place bedrock touches the real process. |
| [bedrocktest](https://pkg.go.dev/github.com/z5labs/bedrock/bedrocktest) | A hermetic fake `Process` for tests that run a whole `App`, in the way `net/http/httptest` serves handlers. |
| [config](https://pkg.go.dev/github.com/z5labs/bedrock/config) | Configuration values composed by chaining, read from the environment of the process, and `config.Struct` to read many at once. |
| [example/httpserver](example/httpserver) | The reference for building on bedrock: a complete HTTP service with the standard library alone, with configuration, graceful shutdown and whole-service tests. |

Library code imports only the standard library.

## fit

[fit](https://pkg.go.dev/github.com/z5labs/bedrock/fit) is request-scoped
failure injection after Netflix's Failure Injection Testing: a request under
test carries a failure scenario as OpenTelemetry baggage, and each injection
point it passes through asks an `Evaluator` which fault to emulate. It is a
nested module with its own `go.mod`, so the OpenTelemetry dependency stays out
of bedrock itself:

```bash
go get github.com/z5labs/bedrock/fit
```

fit owns no wire convention. It declares no baggage key, value grammar, size
limit, signature scheme or expiry; organisations supply their own codecs, a
`Decoder` and, at the edge, an `Encoder`, that decide all of them. The design
is discussed in
[discussion #578](https://github.com/z5labs/bedrock/discussions/578).

## circuit

[circuit](https://pkg.go.dev/github.com/z5labs/bedrock/circuit) is a circuit
breaker for one dependency: the closed, open and half-open state machine, with
no I/O, no background goroutine and no defaults. A call goes through it inline,
in the caller's own code:

```go
t, err := b.Allow(ctx)
if err != nil {
	return err // a *circuit.RejectedError
}
defer t.Release()

err = call(ctx)
t.Done(err)
```

It is a nested module with its own `go.mod`, and its library code imports only
the standard library:

```bash
go get github.com/z5labs/bedrock/circuit
```

circuit ships no policy, classifier or observer. When to open, what counts as a
failure and how changes of state are reported are the caller's decisions, and
transports, interceptors and metrics belong to the framework built on it. The
design is discussed in
[discussion #595](https://github.com/z5labs/bedrock/discussions/595).

## retry

[retry](https://pkg.go.dev/github.com/z5labs/bedrock/retry) retries calls to a
dependency inline, in the caller's own code, by ranging over the attempts of a
call:

```go
var err error
for a := range r.Attempts(ctx) {
	err = call(ctx)
	a.Done(err)
}
return err
```

`Done` classifies each result with a function the caller provides, and returns
why the loop goes on or ends. A `Budget`, after gRPC's retry throttling, limits
the retries of every call that shares it, and `Exponential` backs off with full
jitter.

It is a nested module with its own `go.mod`, and its library code imports only
the standard library:

```bash
go get github.com/z5labs/bedrock/retry
```

retry ships no classifier. What can be retried is the caller's decision, and a
circuit breaker composes with it through the classifier, which returns `Stop`
for the breaker's rejection. Transports, interceptors and metrics belong to the
framework built on it. The design is discussed in
[discussion #606](https://github.com/z5labs/bedrock/discussions/606).

## host

[host](https://pkg.go.dev/github.com/z5labs/bedrock/host) is the network and
the filesystem that a framework hands an App beyond its process. bedrock
defines the contract and never passes a `Host`; each framework declares the
filesystem its users see as an interface that embeds `host.FS` and lists the
methods it needs, and hides the type parameter behind an alias:

```go
type FS interface {
	host.FS

	Open(name string) (*os.File, error)
	Getwd() (string, error)
}

type Host = host.Host[FS]
```

`host.OS()` is the Host of the real machine, and
[hosttest](https://pkg.go.dev/github.com/z5labs/bedrock/host/hosttest) is the
Host of a test: an in-memory network that runs inside a `testing/synctest`
bubble, and a real filesystem confined to the test's temp directory.

It is a nested module with its own `go.mod`, and its library code imports only
the standard library and the module's own packages:

```bash
go get github.com/z5labs/bedrock/host
```

`FS` is sealed, so `host.OSFS` and hosttest's `*FS` are the only filesystems,
and every assignment of one to a framework's interface is checked when it
compiles. The design is discussed in
[discussion #614](https://github.com/z5labs/bedrock/discussions/614).

## Releases

Every module in this repository is released with the same version, after
OpenTelemetry Go: publishing a GitHub release tagged `v0.22.0` releases
`github.com/z5labs/bedrock` at `v0.22.0`, and the
[Release workflow](.github/workflows/release.yaml) tags every nested module
with that version too, so `fit/v0.22.0` is `github.com/z5labs/bedrock/fit` at
`v0.22.0`. Modules are tagged whether or not they changed, so one version
number names a set of modules that were built and tested together.

## Migrating from v0.21.0

bedrock v0.21.0 was built around `Builder`, `Runtime` and `Runner`, with HTTP,
REST and OpenTelemetry runtimes. [MIGRATING.md](MIGRATING.md) says where every
one of its exports went, either to the process model, to humus or to the
standard library, with before and after examples. Its documentation remains
available at
[pkg.go.dev/github.com/z5labs/bedrock@v0.21.0](https://pkg.go.dev/github.com/z5labs/bedrock@v0.21.0).

## Status

The API is in active development and is not yet stable.
