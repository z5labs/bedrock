// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package bedrock_test

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"syscall"
	"testing"

	"github.com/z5labs/bedrock"
	"github.com/z5labs/bedrock/bedrocktest"
)

// The main function of a program built on bedrock is one line. This example
// has no Output comment, so it is compiled but never run: running it would
// exit the test binary.
func Example_main() {
	app := bedrock.AppFunc(func(ctx context.Context, p bedrock.Process) bedrock.ExitStatus {
		ctx, stop := p.NotifyContext(ctx, os.Interrupt)
		defer stop()

		if _, err := fmt.Fprintln(p.Stdout, "hello, world"); err != nil {
			return bedrock.ExitWithCause(1, err)
		}

		<-ctx.Done()
		return bedrock.ExitWithCause(0, context.Cause(ctx))
	})

	// In package main:
	//
	//	func main() { bedrock.Main(app) }
	bedrock.Main(app)
}

// exampleTB stands in for the *testing.T an Example does not have. In a
// test, pass t to bedrocktest.NewProcess instead.
type exampleTB struct {
	testing.TB
	cleanups []func()
}

func (*exampleTB) Helper() {}

func (*exampleTB) Errorf(format string, args ...any) {
	fmt.Printf("test failed: "+format+"\n", args...)
}

func (tb *exampleTB) Cleanup(f func()) {
	tb.cleanups = append(tb.cleanups, f)
}

func (tb *exampleTB) runCleanups() {
	for i := len(tb.cleanups) - 1; i >= 0; i-- {
		tb.cleanups[i]()
	}
}

// An App runs whole inside a test. bedrocktest hands it a hermetic Process,
// so the test sets its arguments and environment, reads what it wrote, and
// asserts on the exit code it returned. Nothing of the real process is read
// or changed.
func ExampleApp() {
	app := bedrock.AppFunc(func(ctx context.Context, p bedrock.Process) bedrock.ExitStatus {
		name, err := p.Environment.Value("NAME")
		if err != nil {
			if _, werr := fmt.Fprintln(p.Stderr, err); werr != nil {
				return bedrock.ExitWithCause(1, errors.Join(err, werr))
			}
			return bedrock.ExitWithCause(1, err)
		}
		if _, err := fmt.Fprintf(p.Stdout, "hello, %s\n", name); err != nil {
			return bedrock.ExitWithCause(1, err)
		}
		return bedrock.Exit(0)
	})

	t := new(exampleTB)
	defer t.runCleanups()

	fp := bedrocktest.NewProcess(t, bedrocktest.WithEnv(map[string]string{"NAME": "gopher"}))
	status := app.Run(context.Background(), fp.Process())

	fmt.Print(fp.Stdout())
	fmt.Println("exit code:", status.Code())
	// Output:
	// hello, gopher
	// exit code: 0
}

// An App shuts down gracefully by deriving its context from
// Process.NotifyContext and returning once a signal cancels it. Returning
// ExitWithCause(0, context.Cause(ctx)) exits successfully while still
// telling a test which signal stopped the App.
func ExampleApp_gracefulShutdown() {
	app := bedrock.AppFunc(func(ctx context.Context, p bedrock.Process) bedrock.ExitStatus {
		ctx, stop := p.NotifyContext(ctx, syscall.SIGTERM)
		defer stop()

		if _, err := fmt.Fprintln(p.Stdout, "serving"); err != nil {
			return bedrock.ExitWithCause(1, err)
		}

		<-ctx.Done()
		if _, err := fmt.Fprintln(p.Stdout, "shutting down"); err != nil {
			return bedrock.ExitWithCause(1, err)
		}
		return bedrock.ExitWithCause(0, context.Cause(ctx))
	})

	t := new(exampleTB)
	defer t.runCleanups()

	fp := bedrocktest.NewProcess(t)
	statusCh := make(chan bedrock.ExitStatus, 1)
	go func() {
		statusCh <- app.Run(context.Background(), fp.Process())
	}()

	// Wait until the App listens for SIGTERM, then send a fake one.
	<-fp.Notified(syscall.SIGTERM)
	fp.Signal(syscall.SIGTERM)
	status := <-statusCh

	fmt.Print(fp.Stdout())
	fmt.Println("exit code:", status.Code())
	fmt.Println("cause:", status.Cause())
	fmt.Println("cancelled:", errors.Is(status.Cause(), context.Canceled))
	// Output:
	// serving
	// shutting down
	// exit code: 0
	// cause: terminated signal received
	// cancelled: true
}

// greet parses its flags with a FlagSet of its own, from p.Args, and
// reports parse errors through flag itself.
func greet(ctx context.Context, p bedrock.Process) bedrock.ExitStatus {
	fs := flag.NewFlagSet(p.Args[0], flag.ContinueOnError)
	fs.SetOutput(p.Stderr) // flag reports the parse error and usage to the user
	n := fs.Int("n", 1, "how many times to greet")
	switch err := fs.Parse(p.Args[1:]); {
	case errors.Is(err, flag.ErrHelp):
		return bedrock.Exit(0)
	case err != nil:
		return bedrock.ExitWithCause(2, err) // cause is for tests; flag already told the user
	}

	for range *n {
		if _, err := fmt.Fprintln(p.Stdout, "hello"); err != nil {
			return bedrock.ExitWithCause(1, err)
		}
	}
	return bedrock.Exit(0)
}

// An App parses its flags from p.Args with flag.ContinueOnError and
// p.Stderr as the FlagSet's output. flag then prints a parse error and the
// usage to the user exactly once, Main prints nothing more, and the cause
// carries the same error for tests. -h is a successful exit.
func ExampleApp_flags() {
	app := bedrock.AppFunc(greet)
	usage := "Usage of " + bedrocktest.ProgramName + ":"

	for _, args := range [][]string{{"-n=2"}, {"-h"}, {"-bogus"}, {"-n=abc"}} {
		t := new(exampleTB)
		fp := bedrocktest.NewProcess(t, bedrocktest.WithArgs(args...))
		status := app.Run(context.Background(), fp.Process())
		t.runCleanups()

		stderr := fp.Stderr()
		fmt.Printf("%s: exit code %d\n", args[0], status.Code())
		fmt.Printf("  stdout: %q\n", fp.Stdout())
		fmt.Printf("  usage printed %d time(s)\n", strings.Count(stderr, usage))
		fmt.Printf("  cause: %v\n", status.Cause())
		if cause := status.Cause(); cause != nil {
			fmt.Printf("  cause printed %d time(s)\n", strings.Count(stderr, cause.Error()))
		}
	}
	// Output:
	// -n=2: exit code 0
	//   stdout: "hello\nhello\n"
	//   usage printed 0 time(s)
	//   cause: <nil>
	// -h: exit code 0
	//   stdout: ""
	//   usage printed 1 time(s)
	//   cause: <nil>
	// -bogus: exit code 2
	//   stdout: ""
	//   usage printed 1 time(s)
	//   cause: flag provided but not defined: -bogus
	//   cause printed 1 time(s)
	// -n=abc: exit code 2
	//   stdout: ""
	//   usage printed 1 time(s)
	//   cause: invalid value "abc" for flag -n: parse error
	//   cause printed 1 time(s)
}
