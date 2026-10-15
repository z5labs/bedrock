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
// # Parsing flags
//
// An App parses its flags from p.Args with a [flag.FlagSet] of its own,
// using flag.ContinueOnError and p.Stderr as the FlagSet's output. flag
// itself prints a parse error followed by the usage to that output, and
// [Main] prints nothing, so the user sees each exactly once. The cause of
// the status carries the same error for tests:
//
//	fs := flag.NewFlagSet(p.Args[0], flag.ContinueOnError)
//	fs.SetOutput(p.Stderr) // flag reports the parse error and usage to the user
//	n := fs.Int("n", 1, "how many times to greet")
//	switch err := fs.Parse(p.Args[1:]); {
//	case errors.Is(err, flag.ErrHelp):
//		return bedrock.Exit(0)
//	case err != nil:
//		return bedrock.ExitWithCause(2, err) // cause is for tests; flag already told the user
//	}
//
// -h and -help print the usage and exit with status 0. The flags example
// of [App] runs this idiom whole.
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
//     process. Parse p.Args[1:] with a FlagSet of your own instead; see
//     Parsing flags above.
//   - An exec.Cmd with a nil Env inherits the real environment; see Child
//     processes above.
//   - http.ProxyFromEnvironment, the Proxy of http.DefaultTransport, reads
//     the proxy variables of the real environment.
//   - OpenTelemetry's resource.Default reads the real environment, and its
//     global providers are process-wide state.
//   - slog.Default writes to the real standard error unless it is replaced,
//     and replacing it is process-wide state.
//
// # Engineering rules
//
// bedrock's engineering rules are inspired by NASA's Power of 10 and
// TigerBeetle's TigerStyle. Neither is a literal standard here: each rule
// is applied, adapted to Go or rejected, and every rule that is applied is
// enforced by a tool in CI rather than by review alone.
//
// The boundary itself is applied. golangci-lint's forbidigo fails any use
// of os.Args, os.Stdin, os.Stdout, os.Stderr, os.Getenv, os.LookupEnv,
// os.Environ, os.Exit, signal.Notify, signal.NotifyContext, flag.Parse or
// flag.CommandLine across the whole module, tests and examples included,
// except in the OS edge, its tests, and the test that checks the fake
// signal delivery of bedrocktest against the real one.
//
// From the Power of 10:
//
//   - Rule 1, simple control flow, is adapted: bedrock uses no goto and no
//     recursion, which review keeps true.
//   - Rule 2, fixed loop bounds, is adapted: every loop ranges over a
//     finite value, and every wait ends when a context is done rather than
//     after a fixed count.
//   - Rule 3, no allocation after initialization, is adapted. Go allocates
//     and collects garbage, so bedrock allocates when a value is built
//     rather than each time it is used: [NewStaticEnvironment] copies its
//     map once and a successful Value only reads it.
//   - Rule 4, short functions, is adapted: functions are kept short, but
//     table-driven tests are exempt, so no tool enforces a line limit.
//   - Rule 5, assertion density, is adapted: a panic asserts a programming
//     error and is never recovered, and [Exit] asserts that its code is in
//     range, but assertions are not counted per function.
//   - Rule 6, smallest scope, is adapted: bedrock has no package-level
//     state, and review keeps every other value at the smallest scope.
//   - Rule 7, check every result, is applied. errcheck runs on test code as
//     well as library code, so no error is discarded silently, and
//     [ExitStatus.Cause] lets a test check why every failure path failed,
//     not only that it failed. Checking parameters is adapted; see
//     Preconditions below.
//   - Rule 8, limited use of the preprocessor, is adapted: Go has none,
//     and build constraints only select a platform.
//   - Rule 9, no function pointers, is rejected. [AppFunc] and the func
//     fields of [Process] are function values, and small functions that
//     compose are how bedrock stays small.
//   - Rule 10, zero warnings, is applied: go vet and golangci-lint run on
//     every pull request, and any finding fails it.
//
// From TigerStyle:
//
//   - Assertions are adapted, as for rule 5: panics assert programming
//     errors, and preconditions are documented rather than checked.
//   - Putting a limit on everything is adapted: an exit code is limited to
//     0..255 and [Exit] checks it, and waits are bounded by contexts.
//   - Static memory allocation is adapted, as for rule 3.
//   - Zero dependencies is adapted: library code imports only the
//     standard library, and tests alone may use third-party modules.
//   - Exhaustive testing is applied as statement coverage: every non-main
//     package is at 100.0%, and CI fails a pull request that leaves any of
//     them below it.
//   - Deterministic simulation testing is not claimed; see Testing claims
//     below.
//
// # Preconditions
//
// The preconditions of a [Process] are documented rather than checked at
// runtime: every field is required, and code that receives a Process uses
// any field without a nil check. The two places that build a Process,
// [OSProcess] and bedrocktest, are guarded instead by tests that use
// reflection to range over every field, so a field added to Process fails
// them until it is set.
//
// # Testing claims
//
// bedrock claims hermetic, black-box testing with deterministic time: an
// App runs whole on a fake Process, the test sees only the status it
// returns and what it writes, and testing/synctest makes its clock
// deterministic. It does not claim deterministic simulation testing. The
// Go scheduler, the network and the filesystem are not simulated, so a run
// cannot be replayed from a seed.
package bedrock
