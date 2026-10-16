# Migrating from v0.21.0

bedrock v0.21.0 was a framework: a `Builder` constructed a `Runtime`, a `Runner`
ran it, and `runtime/http`, `runtime/http/rest` and `runtime/otel` supplied an
HTTP server, a REST DSL and OpenTelemetry setup on top. bedrock is now a
foundation instead. It models an application as an OS process and nothing more:

```go
type App interface {
	Run(ctx context.Context, p bedrock.Process) bedrock.ExitStatus
}

func main() { bedrock.Main(app) }
```

Everything a v0.21.0 export did has gone to one of three places:

- **The process model.** Construction moves to the start of `App.Run`,
  signals come from `Process.NotifyContext`, configuration is read from
  `Process.Environment` with `config.Value`, and failures become an
  `ExitStatus`.
- **[z5labs/humus](https://github.com/z5labs/humus).** The HTTP server
  lifecycle, the REST DSL, OpenTelemetry setup and the panic policy are
  framework features, and humus, the framework built on bedrock, takes them
  over. Each one is tracked by a humus issue, linked from its row.
- **Removed.** What the standard library already does well is left to it,
  and each row names the replacement.

The v0.21.0 documentation remains available at
[pkg.go.dev/github.com/z5labs/bedrock@v0.21.0](https://pkg.go.dev/github.com/z5labs/bedrock@v0.21.0).

- [Before and after](#before-and-after)
  - [A main built from a Runner over a Builder](#a-main-built-from-a-runner-over-a-builder)
  - [A `Run(context.Context) error` runtime](#a-runcontextcontext-error-runtime)
  - [A configuration option](#a-configuration-option)
- [Every export of v0.21.0](#every-export-of-v0210)
- [Packages](#packages)
- [How the list of exports was made](#how-the-list-of-exports-was-made)

## Before and after

### A main built from a Runner over a Builder

At v0.21.0, a main built its component with a `Builder`, ran it with a
`Runner`, and stacked signal handling and panic recovery on the `Runner`:

```go
type Greeter struct {
	greeting string
}

func (g Greeter) Run(ctx context.Context) error {
	fmt.Println(g.greeting)
	<-ctx.Done()
	return nil
}

func build(ctx context.Context) (Greeter, error) {
	greeting, err := config.Read(ctx, config.Default("hello", config.Env("GREETING")))
	if err != nil {
		return Greeter{}, err
	}
	return Greeter{greeting: greeting}, nil
}

func main() {
	runner := bedrock.NotifyOnSignal(
		bedrock.RecoverPanics(bedrock.DefaultRunner[Greeter]()),
		os.Interrupt,
	)
	if err := runner.Run(context.Background(), bedrock.BuilderFunc[Greeter](build)); err != nil {
		log.Fatal(err)
	}
}
```

Now the `App` is the whole program. What the `Builder` did is the start of
`Run`, the signals it listens for are its own choice, and it reports a failure
on its standard error and returns it as the cause of its status. `main` is one
line:

```go
type Greeter struct{}

func (Greeter) Run(ctx context.Context, p bedrock.Process) bedrock.ExitStatus {
	greeting, err := config.Env(p.Environment, "GREETING").Default("hello").Read(ctx)
	if err != nil {
		return fail(p, err)
	}

	ctx, stop := p.NotifyContext(ctx, os.Interrupt)
	defer stop()

	if _, err := fmt.Fprintln(p.Stdout, greeting); err != nil {
		return fail(p, err)
	}
	<-ctx.Done()
	return bedrock.ExitWithCause(0, context.Cause(ctx))
}

// fail writes err to the standard error of p, because bedrock.Main prints
// nothing, and returns it as the cause of a status with exit code 1.
func fail(p bedrock.Process, err error) bedrock.ExitStatus {
	if _, werr := fmt.Fprintln(p.Stderr, err); werr != nil {
		return bedrock.ExitWithCause(1, errors.Join(err, werr))
	}
	return bedrock.ExitWithCause(1, err)
}

func main() { bedrock.Main(Greeter{}) }
```

What changed, piece by piece:

- `BuilderFunc` and `build` became the first lines of `Run`.
- `config.Env("GREETING")` read the environment of the real process.
  `config.Env(p.Environment, "GREETING")` reads the environment the `App` was
  given, so a test sets it with `bedrocktest.WithEnv` instead of `t.Setenv`.
- `NotifyOnSignal` became `p.NotifyContext`, called by the `App` itself.
- `RecoverPanics` is gone. bedrock never recovers a panic: a panic is a
  programming error and crashes the process with exit status 2.
- `log.Fatal` printed the error and exited with status 1. `Main` prints
  nothing, so the `App` writes the error to `p.Stderr` itself, and the cause of
  the status lets a test assert why the `App` failed.
- `DefaultRunner` and `runner.Run` became `bedrock.Main`.

### A `Run(context.Context) error` runtime

An existing runtime with the v0.21.0 `Runtime` shape, such as humus's
`job.App` or `queue.App`, does not have to be rewritten. An adapter makes it a
`bedrock.App`. The adapter makes the two choices that v0.21.0 left to the
`Runner`, and makes them explicitly: which signals stop the runtime, and which
exit code each outcome maps to.

```go
// Runtime is the v0.21.0 shape of a runnable component.
type Runtime interface {
	Run(ctx context.Context) error
}

// AsApp adapts rt to a bedrock.App that stops rt on an interrupt or SIGTERM.
func AsApp(rt Runtime) bedrock.App {
	return bedrock.AppFunc(func(ctx context.Context, p bedrock.Process) bedrock.ExitStatus {
		// An explicit list: calling NotifyContext with no signals means every signal.
		ctx, stop := p.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
		defer stop()

		err := rt.Run(ctx)
		switch {
		case err == nil:
			return bedrock.ExitWithCause(0, context.Cause(ctx))
		case errors.Is(err, context.Canceled) && ctx.Err() != nil:
			// rt stopped because a signal cancelled ctx: a graceful shutdown.
			return bedrock.ExitWithCause(0, context.Cause(ctx))
		default:
			if _, werr := fmt.Fprintln(p.Stderr, err); werr != nil {
				return bedrock.ExitWithCause(1, errors.Join(err, werr))
			}
			return bedrock.ExitWithCause(1, err)
		}
	})
}

func main() { bedrock.Main(AsApp(job)) } // job is the existing Runtime
```

`ExitWithCause(code, err)` keeps the error on the status, which `Main` ignores
and a test reads with `status.Cause()`. A runtime with distinct failure modes
can map each to its own exit code, such as 2 for a usage error, by matching
them with `errors.Is` or `errors.AsType`.

### A configuration option

At v0.21.0, humus took its options as `config.Reader`s and read their defaults
from the environment of the real process. A malformed value fell back to the
default silently, because `runtime/http` read it with `config.MustOr`:

```go
// Port overrides the TCP port. The default is read from HUMUS_REST_PORT (8443).
func Port(r bedrockconfig.Reader[int]) Option {
	return func(o *options) {
		o.port = r
	}
}

func defaultOptions() *options {
	return &options{
		port: bedrockconfig.Default(
			8443,
			bedrockconfig.IntFromString(bedrockconfig.Env("HUMUS_REST_PORT")),
		),
	}
}
```

Now the option takes a `config.Value`, and the default is read inside
`App.Run`, where the environment of the process is available. The zero
`config.Value` is not set, so an option that was never given falls through
`Or` to the environment and then to the default. `Default` recovers only from
a value that is not set, so `HUMUS_REST_PORT=abc` fails startup with
`config HUMUS_REST_PORT: strconv.Atoi: ...` instead of becoming 8443:

```go
// Port overrides the TCP port. The default is read from HUMUS_REST_PORT (8443).
func Port(v config.Value[int]) Option {
	return func(o *options) {
		o.port = v
	}
}

func (a App) Run(ctx context.Context, p bedrock.Process) bedrock.ExitStatus {
	port, err := a.opts.port.
		Or(config.Env(p.Environment, "HUMUS_REST_PORT").Parse(strconv.Atoi)).
		Default(8443).
		Read(ctx)
	if err != nil {
		return fail(p, err)
	}
	// ...
}
```

A caller that overrides the port passes a value, such as
`rest.Port(config.Static(9090))`. With many options, `config.Struct` reads
them all at once and reports every malformed one in a single error. humus
makes this change in
[z5labs/humus#622](https://github.com/z5labs/humus/issues/622).

## Every export of v0.21.0

One row per top-level exported identifier that `go doc -short` reports at
v0.21.0; see [How the list of exports was made](#how-the-list-of-exports-was-made).
Each row says whether the export was **replaced** in the process model,
**moved to humus**, or **removed** in favour of the standard library or a
dependency's own API.

### bedrock

| v0.21.0 | outcome | instead |
| --- | --- | --- |
| `Bind` | removed | Call the next constructor with the result of the first, in plain Go at the start of `App.Run`. |
| `Builder` | replaced | `bedrock.App`. What a `Builder` constructed is constructed at the start of `App.Run`; see [the first example](#a-main-built-from-a-runner-over-a-builder). |
| `BuilderFunc` | removed | An ordinary function returning `(T, error)`, called from `App.Run`. |
| `BuilderOf` | removed | The value itself. |
| `DefaultRunner` | replaced | `bedrock.Main(app)`, which is `app.Run(context.Background(), bedrock.OSProcess())` followed by an exit with its code. |
| `Map` | removed | Call the function on the value, in plain Go. |
| `MemoizeBuilder` | removed | Build the value once in `App.Run` and pass it to everything that needs it, or `sync.OnceValues` to build it lazily. |
| `MustBuild` | removed | Check the error and return `bedrock.ExitWithCause(1, err)`. A panic is only for a programming error. |
| `NotifyOnSignal` | replaced | `Process.NotifyContext`, called by the `App` with the signals it chooses. |
| `RecoverPanics` | moved to humus | bedrock never recovers a panic, which crashes the process with exit status 2. humus decides and documents its own panic policy in [z5labs/humus#624](https://github.com/z5labs/humus/issues/624). |
| `Runner` | replaced | `bedrock.Main`. A `Runner` that wrapped another is a `bedrock.App` that wraps another. |
| `RunnerFunc` | replaced | `bedrock.AppFunc`, wrapping the `App` it decorates. |
| `Runtime` | replaced | `bedrock.App`, whose `Run` takes a `bedrock.Process` and returns a `bedrock.ExitStatus`; see [the adapter](#a-runcontextcontext-error-runtime). |
| `RuntimeFunc` | replaced | `bedrock.AppFunc`. |

### bedrock/config

| v0.21.0 | outcome | instead |
| --- | --- | --- |
| `Bind` | replaced | `Value.FlatMap`. |
| `BoolFromString` | replaced | `.Parse(strconv.ParseBool)`. |
| `Default` | replaced | `Value.Default`, which recovers only from a value that is not set. |
| `DurationFromString` | replaced | `.Parse(time.ParseDuration)`. |
| `EmptyReader` | replaced | The zero `config.Value[T]{}`, which reads as `config.ErrNotFound`. |
| `Env` | replaced | `config.Env(p.Environment, name)`, which reads the environment the `App` was given. |
| `ErrValueNotSet` | replaced | `config.ErrNotFound`. |
| `Float64FromString` | replaced | `.Parse(config.ParseFloat64)`. |
| `Int64FromBytes` | removed | `binary.Read` from `encoding/binary`, inside `config.Func` or `Value.Parse`. |
| `Int64FromString` | replaced | `.Parse(config.ParseInt64)`. |
| `IntFromString` | replaced | `.Parse(strconv.Atoi)`. |
| `Map` | replaced | `Value.Map` for a function that cannot fail, `Value.Parse` for one that can. |
| `Must` | removed | `Value.Read`, returning its error as the cause of a non-zero `bedrock.ExitStatus`. A configuration failure is an input error, not a programming error. |
| `MustOr` | removed | `Value.Default(d)`, which recovers only from a value that is not set, so a malformed value is an error instead of the default. |
| `Or` | replaced | `Value.Or`, chained: `v1.Or(v2).Or(v3)`. |
| `Read` | replaced | `Value.Read`. |
| `ReadFile` | removed | `fs.ReadFile` from `io/fs`, over an `fs.FS` the `App` is given, inside `config.Func`. |
| `Reader` | replaced | `config.Source[T]`, the interface a source satisfies, and `config.Value[T]`, the concrete type with the combinators. |
| `ReaderFunc` | replaced | `config.Func(name, fn)`, and `config.From(name, src)` for any `config.Source`. |
| `ReaderOf` | replaced | `config.Static`. |
| `Value` | removed | `(T, error)`, as `Value.Read` returns it. A value that is not set is an error satisfying `errors.Is(err, config.ErrNotFound)`. |
| `ValueOf` | removed | Return `(v, nil)` from the function given to `config.Func`. |

### bedrock/runtime/http

humus takes over the HTTP server lifecycle in
[z5labs/humus#621](https://github.com/z5labs/humus/issues/621).
[example/httpserver](example/httpserver) is the reference it follows: a
complete service built with `net/http` alone.

| v0.21.0 | outcome | instead |
| --- | --- | --- |
| `Build` | moved to humus | [z5labs/humus#621](https://github.com/z5labs/humus/issues/621). Without humus, an `http.Server` served from `App.Run` and shut down within a grace period, as in [example/httpserver](example/httpserver). |
| `BuildTCPListener` | removed | `net.Listen("tcp", addr)`. |
| `BuildTLSListener` | removed | `tls.NewListener` from `crypto/tls`. |
| `DisableGeneralOptionsHandler` | removed | The `http.Server.DisableGeneralOptionsHandler` field. |
| `IdleTimeout` | removed | The `http.Server.IdleTimeout` field. |
| `MaxHeaderBytes` | removed | The `http.Server.MaxHeaderBytes` field. |
| `ReadHeaderTimeout` | removed | The `http.Server.ReadHeaderTimeout` field. |
| `ReadTimeout` | removed | The `http.Server.ReadTimeout` field. |
| `Runtime` | moved to humus | [z5labs/humus#621](https://github.com/z5labs/humus/issues/621): humus's rest `App.Run` listens, serves and shuts down gracefully. |
| `Server` | removed | `http.Server` from `net/http`. |
| `ServerOption` | removed | Set the fields of `http.Server` directly, reading their values with `config`. |
| `WriteTimeout` | removed | The `http.Server.WriteTimeout` field. |

### bedrock/runtime/http/rest

The REST endpoint DSL and its OpenAPI generation move into humus unchanged in
[z5labs/humus#620](https://github.com/z5labs/humus/issues/620), apart from
`Build`, which becomes `NewHandler`.

| v0.21.0 | outcome | instead |
| --- | --- | --- |
| `APIDescription` | moved to humus | [z5labs/humus#620](https://github.com/z5labs/humus/issues/620) |
| `Build` | moved to humus | [z5labs/humus#620](https://github.com/z5labs/humus/issues/620), as `NewHandler(opts...) (http.Handler, error)`. |
| `CatchAll` | moved to humus | [z5labs/humus#620](https://github.com/z5labs/humus/issues/620) |
| `DELETE` | moved to humus | [z5labs/humus#620](https://github.com/z5labs/humus/issues/620) |
| `DefaultValue` | moved to humus | [z5labs/humus#620](https://github.com/z5labs/humus/issues/620) |
| `EmptyBody` | moved to humus | [z5labs/humus#620](https://github.com/z5labs/humus/issues/620) |
| `Endpoint` | moved to humus | [z5labs/humus#620](https://github.com/z5labs/humus/issues/620) |
| `EndpointDescription` | moved to humus | [z5labs/humus#620](https://github.com/z5labs/humus/issues/620) |
| `Enum` | moved to humus | [z5labs/humus#620](https://github.com/z5labs/humus/issues/620) |
| `ErrorJSON` | moved to humus | [z5labs/humus#620](https://github.com/z5labs/humus/issues/620) |
| `GET` | moved to humus | [z5labs/humus#620](https://github.com/z5labs/humus/issues/620) |
| `HeaderParam` | moved to humus | [z5labs/humus#620](https://github.com/z5labs/humus/issues/620) |
| `MarkDeprecated` | moved to humus | [z5labs/humus#620](https://github.com/z5labs/humus/issues/620) |
| `MaxLength` | moved to humus | [z5labs/humus#620](https://github.com/z5labs/humus/issues/620) |
| `Maximum` | moved to humus | [z5labs/humus#620](https://github.com/z5labs/humus/issues/620) |
| `MinLength` | moved to humus | [z5labs/humus#620](https://github.com/z5labs/humus/issues/620) |
| `Minimum` | moved to humus | [z5labs/humus#620](https://github.com/z5labs/humus/issues/620) |
| `OperationID` | moved to humus | [z5labs/humus#620](https://github.com/z5labs/humus/issues/620) |
| `Option` | moved to humus | [z5labs/humus#620](https://github.com/z5labs/humus/issues/620) |
| `Optional` | moved to humus | [z5labs/humus#620](https://github.com/z5labs/humus/issues/620) |
| `PATCH` | moved to humus | [z5labs/humus#620](https://github.com/z5labs/humus/issues/620) |
| `POST` | moved to humus | [z5labs/humus#620](https://github.com/z5labs/humus/issues/620) |
| `PUT` | moved to humus | [z5labs/humus#620](https://github.com/z5labs/humus/issues/620) |
| `Param` | moved to humus | [z5labs/humus#620](https://github.com/z5labs/humus/issues/620) |
| `ParamDescription` | moved to humus | [z5labs/humus#620](https://github.com/z5labs/humus/issues/620) |
| `ParamExample` | moved to humus | [z5labs/humus#620](https://github.com/z5labs/humus/issues/620) |
| `ParamFrom` | moved to humus | [z5labs/humus#620](https://github.com/z5labs/humus/issues/620) |
| `ParamOption` | moved to humus | [z5labs/humus#620](https://github.com/z5labs/humus/issues/620) |
| `PathParam` | moved to humus | [z5labs/humus#620](https://github.com/z5labs/humus/issues/620) |
| `Pattern` | moved to humus | [z5labs/humus#620](https://github.com/z5labs/humus/issues/620) |
| `QueryParam` | moved to humus | [z5labs/humus#620](https://github.com/z5labs/humus/issues/620) |
| `ReadBinary` | moved to humus | [z5labs/humus#620](https://github.com/z5labs/humus/issues/620) |
| `ReadFormField` | moved to humus | [z5labs/humus#620](https://github.com/z5labs/humus/issues/620) |
| `ReadFormFile` | moved to humus | [z5labs/humus#620](https://github.com/z5labs/humus/issues/620) |
| `ReadJSON` | moved to humus | [z5labs/humus#620](https://github.com/z5labs/humus/issues/620) |
| `Request` | moved to humus | [z5labs/humus#620](https://github.com/z5labs/humus/issues/620) |
| `Required` | moved to humus | [z5labs/humus#620](https://github.com/z5labs/humus/issues/620) |
| `Route` | moved to humus | [z5labs/humus#620](https://github.com/z5labs/humus/issues/620) |
| `SpecPath` | moved to humus | [z5labs/humus#620](https://github.com/z5labs/humus/issues/620) |
| `Summary` | moved to humus | [z5labs/humus#620](https://github.com/z5labs/humus/issues/620) |
| `Tags` | moved to humus | [z5labs/humus#620](https://github.com/z5labs/humus/issues/620) |
| `Title` | moved to humus | [z5labs/humus#620](https://github.com/z5labs/humus/issues/620) |
| `ValidationError` | moved to humus | [z5labs/humus#620](https://github.com/z5labs/humus/issues/620) |
| `Version` | moved to humus | [z5labs/humus#620](https://github.com/z5labs/humus/issues/620) |
| `WriteBinary` | moved to humus | [z5labs/humus#620](https://github.com/z5labs/humus/issues/620) |
| `WriteJSON` | moved to humus | [z5labs/humus#620](https://github.com/z5labs/humus/issues/620) |

### bedrock/runtime/otel

humus builds its OpenTelemetry providers with direct SDK calls inside
`App.Run` in [z5labs/humus#623](https://github.com/z5labs/humus/issues/623).
Most of these exports were one-line wrappers, and each row names the SDK call
that replaces it.

| v0.21.0 | outcome | instead |
| --- | --- | --- |
| `BuildBatchLogProcessor` | moved to humus | [z5labs/humus#623](https://github.com/z5labs/humus/issues/623); `sdklog.NewBatchProcessor`. |
| `BuildBatchSpanProcessor` | moved to humus | [z5labs/humus#623](https://github.com/z5labs/humus/issues/623); `sdktrace.NewBatchSpanProcessor`. |
| `BuildLoggerProvider` | moved to humus | [z5labs/humus#623](https://github.com/z5labs/humus/issues/623); `sdklog.NewLoggerProvider`. |
| `BuildMeterProvider` | moved to humus | [z5labs/humus#623](https://github.com/z5labs/humus/issues/623); `sdkmetric.NewMeterProvider`. |
| `BuildPeriodicReader` | moved to humus | [z5labs/humus#623](https://github.com/z5labs/humus/issues/623); `sdkmetric.NewPeriodicReader`. |
| `BuildRuntime` | moved to humus | [z5labs/humus#623](https://github.com/z5labs/humus/issues/623): humus registers its providers, runs, and shuts them down within the grace period, inline in `App.Run`. |
| `BuildTraceIDRatioBasedSampler` | moved to humus | [z5labs/humus#623](https://github.com/z5labs/humus/issues/623); `sdktrace.TraceIDRatioBased`. |
| `BuildTracerProvider` | moved to humus | [z5labs/humus#623](https://github.com/z5labs/humus/issues/623); `sdktrace.NewTracerProvider`. |
| `Runtime` | moved to humus | [z5labs/humus#623](https://github.com/z5labs/humus/issues/623); the register, run and shutdown lifecycle is inlined in humus's `App.Run`. |
| `RuntimeOption` | moved to humus | [z5labs/humus#623](https://github.com/z5labs/humus/issues/623); the only option was the shutdown grace period. |
| `RuntimeOptions` | moved to humus | [z5labs/humus#623](https://github.com/z5labs/humus/issues/623); the only option was the shutdown grace period. |
| `ShutdownGracePeriod` | moved to humus | [z5labs/humus#623](https://github.com/z5labs/humus/issues/623); `context.WithTimeout` around the providers' `Shutdown`. |

### bedrock/runtime/otel/noop

| v0.21.0 | outcome | instead |
| --- | --- | --- |
| `BuildLogExporter` | moved to humus | [z5labs/humus#623](https://github.com/z5labs/humus/issues/623); the logger provider of `go.opentelemetry.io/otel/log/noop`. |
| `BuildMetricExporter` | moved to humus | [z5labs/humus#623](https://github.com/z5labs/humus/issues/623); the meter provider of `go.opentelemetry.io/otel/metric/noop`. |
| `BuildSpanExporter` | moved to humus | [z5labs/humus#623](https://github.com/z5labs/humus/issues/623); the tracer provider of `go.opentelemetry.io/otel/trace/noop`. |
| `LogExporter` | moved to humus | [z5labs/humus#623](https://github.com/z5labs/humus/issues/623); the logger provider of `go.opentelemetry.io/otel/log/noop`. |
| `MetricExporter` | moved to humus | [z5labs/humus#623](https://github.com/z5labs/humus/issues/623); the meter provider of `go.opentelemetry.io/otel/metric/noop`. |
| `SpanExporter` | moved to humus | [z5labs/humus#623](https://github.com/z5labs/humus/issues/623); the tracer provider of `go.opentelemetry.io/otel/trace/noop`. |

### bedrock/runtime/otel/otlp

| v0.21.0 | outcome | instead |
| --- | --- | --- |
| `BuildGrpcLogExporter` | moved to humus | [z5labs/humus#623](https://github.com/z5labs/humus/issues/623); `otlploggrpc.New` with `otlploggrpc.WithGRPCConn`. |
| `BuildGrpcMetricExporter` | moved to humus | [z5labs/humus#623](https://github.com/z5labs/humus/issues/623); `otlpmetricgrpc.New` with `otlpmetricgrpc.WithGRPCConn`. |
| `BuildGrpcSpanExporter` | moved to humus | [z5labs/humus#623](https://github.com/z5labs/humus/issues/623); `otlptracegrpc.New` with `otlptracegrpc.WithGRPCConn`. |
| `BuildHttpLogExporter` | moved to humus | [z5labs/humus#623](https://github.com/z5labs/humus/issues/623); `otlploghttp.New` with `otlploghttp.WithEndpoint` and `otlploghttp.WithHTTPClient`. |
| `BuildHttpMetricExporter` | moved to humus | [z5labs/humus#623](https://github.com/z5labs/humus/issues/623); `otlpmetrichttp.New` with `otlpmetrichttp.WithEndpoint` and `otlpmetrichttp.WithHTTPClient`. |
| `BuildHttpSpanExporter` | moved to humus | [z5labs/humus#623](https://github.com/z5labs/humus/issues/623); `otlptracehttp.New` with `otlptracehttp.WithEndpoint` and `otlptracehttp.WithHTTPClient`. |

### bedrock/runtime/otel/stdout

The writer of each exporter is now `p.Stdout`, or another stream of the
process, rather than one the exporter reaches for itself.

| v0.21.0 | outcome | instead |
| --- | --- | --- |
| `BuildLogExporter` | moved to humus | [z5labs/humus#623](https://github.com/z5labs/humus/issues/623); `stdoutlog.New(stdoutlog.WithWriter(p.Stdout))`. |
| `BuildMetricExporter` | moved to humus | [z5labs/humus#623](https://github.com/z5labs/humus/issues/623); `stdoutmetric.New(stdoutmetric.WithWriter(p.Stdout))`. |
| `BuildSpanExporter` | moved to humus | [z5labs/humus#623](https://github.com/z5labs/humus/issues/623); `stdouttrace.New(stdouttrace.WithWriter(p.Stdout))`. |

## Packages

| v0.21.0 package | outcome | instead |
| --- | --- | --- |
| `internal/fixedpool` | removed | `sync.WaitGroup.Go` with `context.WithCancelCause` to stop the other tasks on the first failure, and `errors.Join` to collect the errors, as [example/httpserver](example/httpserver) does. It was internal, so nothing outside bedrock imported it. Unlike `fixedpool`, nothing recovers a panic in a task. |
| `example/http/rest/ecommerce`, with `app`, `endpoint` and `services/cart` | replaced | [example/httpserver](example/httpserver), a complete HTTP service built on the process model with `net/http` alone. A service built with the REST DSL is humus's shoppingcart example, which moves to the DSL's new home in [z5labs/humus#620](https://github.com/z5labs/humus/issues/620). |

## How the list of exports was made

Every row of [Every export of v0.21.0](#every-export-of-v0210) is one line of
`go doc -short` at v0.21.0. This lists them, one `package identifier` pair per
line, from a checkout of the tag:

```bash
git worktree add --detach /tmp/bedrock-v0.21.0 v0.21.0
cd /tmp/bedrock-v0.21.0
for p in . config runtime/http runtime/http/rest runtime/otel runtime/otel/noop runtime/otel/otlp runtime/otel/stdout; do
	go doc -short "./$p" |
		sed -nE "s#^[[:space:]]*(func|type|var|const) ([A-Za-z0-9_]+).*#bedrock/$p \2#p"
done | sed 's#^bedrock/\. #bedrock #' | sort > /tmp/exports.txt
```

This lists the rows of this file in the same form, and the two lists are
identical, so every export has exactly one row:

```bash
awk '/^## Packages/{exit} /^## Every export/{on=1} on && /^### /{pkg=$2} pkg && /^\| `/{split($0, f, "`"); print pkg, f[2]}' MIGRATING.md |
	sort | diff /tmp/exports.txt -
```
