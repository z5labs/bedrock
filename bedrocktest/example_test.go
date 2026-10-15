// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package bedrocktest_test

import (
	"context"
	"flag"
	"fmt"
	"syscall"
	"testing"

	"github.com/z5labs/bedrock"
	"github.com/z5labs/bedrock/bedrocktest"
)

// greeter is a whole App: it parses its arguments, reads its environment,
// writes to stdout and shuts down on SIGTERM.
var greeter = bedrock.AppFunc(func(ctx context.Context, p bedrock.Process) bedrock.ExitStatus {
	fs := flag.NewFlagSet(p.Args[0], flag.ContinueOnError)
	fs.SetOutput(p.Stderr)
	name := fs.String("name", "world", "who to greet")
	if err := fs.Parse(p.Args[1:]); err != nil {
		return bedrock.ExitWithCause(2, err)
	}

	greeting, err := p.Environment.Value("GREETING")
	if err != nil {
		greeting = "hello"
	}

	ctx, stop := p.NotifyContext(ctx, syscall.SIGTERM)
	defer stop()

	if _, err := fmt.Fprintf(p.Stdout, "%s, %s\n", greeting, *name); err != nil {
		return bedrock.ExitWithCause(1, err)
	}

	<-ctx.Done()
	return bedrock.ExitWithCause(0, context.Cause(ctx))
})

// exampleTB stands in for the *testing.T an Example does not have. In a
// test, pass t to NewProcess instead.
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

// NewProcess runs a whole App hermetically: its arguments, environment,
// streams and signals all come from the fake, and nothing of the real
// process is read or changed.
func ExampleNewProcess() {
	t := new(exampleTB)
	defer t.runCleanups()

	fp := bedrocktest.NewProcess(t,
		bedrocktest.WithArgs("-name", "gopher"),
		bedrocktest.WithEnv(map[string]string{"GREETING": "hi"}),
	)

	statusCh := make(chan bedrock.ExitStatus, 1)
	go func() {
		statusCh <- greeter.Run(context.Background(), fp.Process())
	}()

	<-fp.Notified(syscall.SIGTERM)
	fp.Signal(syscall.SIGTERM)
	status := <-statusCh

	fmt.Print(fp.Stdout())
	fmt.Println("exit code:", status.Code())
	fmt.Println("cause:", status.Cause())
	// Output:
	// hi, gopher
	// exit code: 0
	// cause: terminated signal received
}
