// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package config_test

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/z5labs/bedrock"
	"github.com/z5labs/bedrock/bedrocktest"
	"github.com/z5labs/bedrock/config"
)

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

// server reads its configuration from the environment of the process it is
// run on, never from the environment of the current OS process.
var server = bedrock.AppFunc(func(ctx context.Context, p bedrock.Process) bedrock.ExitStatus {
	port, err := config.Env(p.Environment, "PORT").Parse(strconv.Atoi).Default(8080).Read(ctx)
	if err != nil {
		if _, werr := fmt.Fprintln(p.Stderr, err); werr != nil {
			return bedrock.ExitWithCause(1, werr)
		}
		return bedrock.ExitWithCause(2, err)
	}

	host, err := config.Env(p.Environment, "HOST").Or(config.Env(p.Environment, "HOSTNAME")).Default("localhost").Read(ctx)
	if err != nil {
		return bedrock.ExitWithCause(2, err)
	}

	if _, err := fmt.Fprintf(p.Stdout, "listening on %s:%d\n", host, port); err != nil {
		return bedrock.ExitWithCause(1, err)
	}
	return bedrock.Exit(0)
})

// An App reads its configuration through p.Environment inside Run, so a
// test sets the environment of a bedrocktest Process rather than of the
// test binary. A missing value takes its default, and a malformed one is an
// error rather than the default.
func Example() {
	t := new(exampleTB)
	defer t.runCleanups()

	run := func(env map[string]string) {
		fp := bedrocktest.NewProcess(t, bedrocktest.WithEnv(env))
		status := server.Run(context.Background(), fp.Process())
		fmt.Print(fp.Stdout(), fp.Stderr())
		fmt.Println("exit code:", status.Code())
	}

	run(map[string]string{"HOSTNAME": "example.com"})
	run(map[string]string{"PORT": "9090"})
	run(map[string]string{"PORT": "abc"})
	// Output:
	// listening on example.com:8080
	// exit code: 0
	// listening on localhost:9090
	// exit code: 0
	// config PORT: strconv.Atoi: parsing "abc": invalid syntax
	// exit code: 2
}

// Default recovers only from a value that is not set. A value that is set
// but malformed fails the read rather than falling back.
func ExampleValue_Default() {
	ctx := context.Background()
	for _, env := range []map[string]string{{}, {"PORT": "9090"}, {"PORT": ""}, {"PORT": "abc"}} {
		port, err := config.Env(bedrock.NewStaticEnvironment(env), "PORT").Parse(strconv.Atoi).Default(8080).Read(ctx)
		fmt.Println(port, err)
	}
	// Output:
	// 8080 <nil>
	// 9090 <nil>
	// 0 config PORT: strconv.Atoi: parsing "": invalid syntax
	// 0 config PORT: strconv.Atoi: parsing "abc": invalid syntax
}

// FlatMap reads a value whose source depends on another value, here the
// name of the variable that holds a connection string.
func ExampleValue_FlatMap() {
	env := bedrock.NewStaticEnvironment(map[string]string{
		"DB":          "REPLICA_DSN",
		"REPLICA_DSN": "postgres://replica",
	})

	dsn, err := config.Env(env, "DB").FlatMap(func(name string) config.Value[string] {
		return config.Env(env, name)
	}).Read(context.Background())
	fmt.Println(dsn, err)
	// Output:
	// postgres://replica <nil>
}

// OneOf restricts a value to a fixed set.
func ExampleOneOf() {
	env := bedrock.NewStaticEnvironment(map[string]string{"LOG_LEVEL": "trace"})

	_, err := config.Env(env, "LOG_LEVEL").Parse(config.OneOf("debug", "info", "warn", "error")).Read(context.Background())
	fmt.Println(err)
	// Output:
	// config LOG_LEVEL: trace is not one of [debug info warn error]
}

// Validate checks a value without changing it. Chained after Default, it
// checks the default too, and an error is named after the variable whichever
// of the two the value came from.
func ExampleValue_Validate() {
	nonNegative := func(d time.Duration) error {
		if d < 0 {
			return errors.New("must not be negative")
		}
		return nil
	}

	read := func(env map[string]string, d time.Duration) {
		timeout, err := config.Env(bedrock.NewStaticEnvironment(env), "READ_TIMEOUT").
			Parse(time.ParseDuration).
			Default(d).
			Validate(nonNegative).
			Read(context.Background())
		fmt.Println(timeout, err)
	}

	read(map[string]string{"READ_TIMEOUT": "-5s"}, 5*time.Second)
	read(map[string]string{}, -5*time.Second)
	read(map[string]string{"READ_TIMEOUT": "30s"}, 5*time.Second)
	// Output:
	// 0s config READ_TIMEOUT: must not be negative
	// 0s config READ_TIMEOUT: must not be negative
	// 30s <nil>
}

// Settings is the configuration of the service in ExampleStruct.
type Settings struct {
	Addr        string
	ReadTimeout time.Duration
	Mode        string
}

// service builds its Settings from many values and reports every one that
// is missing or malformed together, rather than one per restart.
var service = bedrock.AppFunc(func(ctx context.Context, p bedrock.Process) bedrock.ExitStatus {
	cfg := config.Struct(func(r *config.Resolver) Settings {
		env := func(name string) config.Value[string] { return config.Env(p.Environment, name) }
		return Settings{
			Addr:        r.Get(env("ADDR")),
			ReadTimeout: r.Get(env("READ_TIMEOUT").Parse(time.ParseDuration).Default(5 * time.Second)),
			Mode:        r.Get(env("MODE").Parse(config.OneOf("dev", "prod")).Default("dev")),
		}
	})

	settings, err := cfg.Read(ctx)
	if err != nil {
		if _, werr := fmt.Fprintln(p.Stderr, err); werr != nil {
			return bedrock.ExitWithCause(1, werr)
		}
		return bedrock.ExitWithCause(2, err)
	}

	if _, err := fmt.Fprintf(p.Stdout, "%+v\n", settings); err != nil {
		return bedrock.ExitWithCause(1, err)
	}
	return bedrock.Exit(0)
})

// Struct reads every value it is built from, so with ADDR missing and
// READ_TIMEOUT malformed, both failures are reported in one error.
func ExampleStruct() {
	t := new(exampleTB)
	defer t.runCleanups()

	run := func(env map[string]string) {
		fp := bedrocktest.NewProcess(t, bedrocktest.WithEnv(env))
		status := service.Run(context.Background(), fp.Process())
		fmt.Print(fp.Stdout(), fp.Stderr())
		fmt.Println("exit code:", status.Code())
	}

	run(map[string]string{"READ_TIMEOUT": "30"})
	run(map[string]string{"ADDR": ":8080", "READ_TIMEOUT": "30s"})
	// Output:
	// config ADDR: not found
	// config READ_TIMEOUT: time: missing unit in duration "30"
	// exit code: 2
	// {Addr::8080 ReadTimeout:30s Mode:dev}
	// exit code: 0
}
