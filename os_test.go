// Copyright (c) 2023 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package bedrock_test

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"testing"

	"github.com/z5labs/bedrock"

	"github.com/stretchr/testify/require"
)

// mainAppEnv names the environment variable that tells a re-executed test
// binary which of mainApps to run through bedrock.Main.
const mainAppEnv = "BEDROCK_TEST_MAIN_APP"

var errMainCause = errors.New("a cause bedrock.Main must not print")

var mainApps = map[string]bedrock.App{
	"zero": bedrock.AppFunc(func(bedrock.Context) bedrock.ExitStatus {
		return bedrock.ExitStatus{}
	}),
	"exit-7": bedrock.AppFunc(func(bedrock.Context) bedrock.ExitStatus {
		return bedrock.Exit(7)
	}),
	"exit-7-with-cause": bedrock.AppFunc(func(bedrock.Context) bedrock.ExitStatus {
		return bedrock.ExitWithCause(7, errMainCause)
	}),
	"exit-300": bedrock.AppFunc(func(bedrock.Context) bedrock.ExitStatus {
		return bedrock.Exit(300)
	}),
}

// TestMainHelperProcess is not a test on its own. TestMainExitsWithTheAppCode
// re-executes the test binary to run only this function, with mainAppEnv
// naming the App to hand to bedrock.Main, which then exits the child.
func TestMainHelperProcess(t *testing.T) {
	if _, ok := os.LookupEnv(mainAppEnv); !ok {
		t.Skip("runs only as a child process of TestMainExitsWithTheAppCode")
	}

	// The App reads its name through the Context Main hands it, which
	// shows Main runs it on the environment of the real process.
	bedrock.Main(bedrock.AppFunc(func(ctx bedrock.Context) bedrock.ExitStatus {
		name, err := ctx.Environment.Value(mainAppEnv)
		if err != nil {
			panic(err)
		}
		app, ok := mainApps[name]
		if !ok {
			panic("unknown main app: " + name)
		}
		return app.Run(ctx)
	}))
}

func TestMainExitsWithTheAppCode(t *testing.T) {
	t.Parallel()

	exe, err := os.Executable()
	require.NoError(t, err)

	testCases := []struct {
		app         string
		code        int
		emptyStderr bool
	}{
		{app: "zero", code: 0, emptyStderr: true},
		{app: "exit-7", code: 7, emptyStderr: true},
		{app: "exit-7-with-cause", code: 7, emptyStderr: true},
		// Exit(300) panics inside the App, and the unrecovered panic
		// crashes the process with exit status 2.
		{app: "exit-300", code: 2},
	}

	for _, tc := range testCases {
		t.Run(tc.app, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer
			cmd := exec.Command(exe, "-test.run=^TestMainHelperProcess$")
			// Inheriting os.Environ() passes GOCOVERDIR to the child, so the
			// coverage of Main in the child merges into the parent's.
			cmd.Env = append(os.Environ(), mainAppEnv+"="+tc.app)
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			err := cmd.Run()
			if tc.code == 0 {
				require.NoError(t, err, "stderr: %s", stderr.String())
			} else {
				var exitErr *exec.ExitError
				require.ErrorAs(t, err, &exitErr)
			}
			require.Equal(t, tc.code, cmd.ProcessState.ExitCode(), "stderr: %s", stderr.String())

			if tc.emptyStderr {
				require.Empty(t, stderr.String())
			} else {
				require.Contains(t, stderr.String(), strconv.Itoa(300))
			}
		})
	}
}

func TestOsEnvironment(t *testing.T) {
	t.Parallel()

	t.Run("returns a ValueNotFoundError naming a variable that is not set", func(t *testing.T) {
		t.Parallel()

		const name = "BEDROCK_TEST_VARIABLE_THAT_IS_NEVER_SET"
		_, err := bedrock.OsEnvironment{}.Value(name)

		var notFound bedrock.ValueNotFoundError
		require.ErrorAs(t, err, &notFound)
		require.Equal(t, name, notFound.Name)
	})
}
