// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package bedrock models an application as an OS process.
//
// # The boundary
//
// exec hands a process its arguments, its environment, its standard
// streams and the delivery of its signals, and takes back an exit status.
// bedrock hands an [App] the same things as one [Process] value, together
// with a root context, and takes back an [ExitStatus]:
//
//	Run(ctx context.Context, p Process) ExitStatus
//
// That call is the boundary. An App never reads os.Args, os.Getenv,
// os.Stdin or the other standard streams, never calls signal.Notify and
// never calls os.Exit. Everything it learns from its process comes through
// p, and the only thing it does to its process is return a status. So an
// App runs whole inside a test, on a hand-built Process.
//
// # The one-line main
//
// [Main] is the only place bedrock touches the real process. It runs an App
// on [OSProcess] with context.Background() and exits with the code of the
// status it returns:
//
//	func main() { bedrock.Main(app) }
//
// Main exits with the status code and never prints the cause.
//
// # Testing
//
// A test builds a Process from its own values, such as a
// [StaticEnvironment] and in-memory streams, runs the App with
// t.Context() as its root context, and asserts on the returned status and
// on what the App wrote. Such a test never changes the environment of the
// test binary, so it can run in parallel.
//
// The cause of an ExitStatus exists so that a negative test can assert why
// an App failed, for example with errors.Is. Nothing reads it in
// production. Anything a user should see, the App writes to p.Stderr.
//
// # Beyond the boundary
//
// bedrock owns only what exec hands a process. The network, the
// filesystem, the clock and randomness belong to the App or to the
// framework built on bedrock, which passes them in explicitly. In tests,
// time comes from testing/synctest, and randomness from testing/cryptotest
// or a seeded source.
//
// # Child processes
//
// An [Environment] offers lookup only, so it cannot be listed the way
// os.Environ lists the real environment. An App that starts a child
// process with os/exec must build the child's exec.Cmd.Env from the keys
// it knows, reading each through Environment.Value. It must never leave
// Env nil, because a nil Env inherits the environment of the real process
// and so reaches around the boundary.
//
// # Panics
//
// A panic is an assertion that a programming error happened. bedrock never
// recovers a panic, so a panic in an App crashes the process with exit
// status 2.
//
// # Ambient leaks
//
// Some of the standard library and of common dependencies read the real
// process behind an App's back. Avoid them, or configure them from p:
//
//   - flag.CommandLine and flag.ExitOnError read os.Args and exit the
//     process. Parse p.Args[1:] with a FlagSet of your own instead, using
//     flag.ContinueOnError.
//   - An exec.Cmd with a nil Env inherits the real environment; see Child
//     processes above.
//   - http.ProxyFromEnvironment, the Proxy of http.DefaultTransport, reads
//     the proxy variables of the real environment.
//   - OpenTelemetry's resource.Default reads the real environment, and its
//     global providers are process-wide state.
//   - slog.Default writes to the real standard error unless it is replaced,
//     and replacing it is process-wide state.
package bedrock
