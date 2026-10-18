// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package hosttest_test

import (
	"fmt"
	"os"
	"testing"
)

// exampleTB stands in for the *testing.T an Example does not have. In a
// test, pass t to hosttest.New instead. It implements only the methods
// hosttest calls: Helper, Errorf, Fatalf, Cleanup and TempDir. Any other
// method panics on the nil testing.TB.
//
// It lives in a file of its own, outside the example files, so that the
// fetch example stays a whole-file example.
type exampleTB struct {
	testing.TB
	cleanups []func()
}

func (*exampleTB) Helper() {}

func (*exampleTB) Errorf(format string, args ...any) {
	fmt.Printf("test failed: "+format+"\n", args...)
}

func (*exampleTB) Fatalf(format string, args ...any) {
	panic(fmt.Sprintf("test failed: "+format, args...))
}

func (tb *exampleTB) Cleanup(f func()) {
	tb.cleanups = append(tb.cleanups, f)
}

// TempDir makes a new directory, removed by runCleanups, as t.TempDir's is
// removed when the test ends.
func (tb *exampleTB) TempDir() string {
	dir, err := os.MkdirTemp("", "hosttest-example-")
	if err != nil {
		tb.Fatalf("TempDir: %v", err)
	}
	tb.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func (tb *exampleTB) runCleanups() {
	for i := len(tb.cleanups) - 1; i >= 0; i-- {
		tb.cleanups[i]()
	}
}
