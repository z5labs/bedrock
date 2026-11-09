// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package pki_test

import (
	"fmt"
	"testing"
)

// exampleTB stands in for the *testing.T an Example does not have. In a
// test, pass t to pkitest instead. It implements only the methods pkitest
// calls: Helper and Fatalf. Any other method panics on the nil testing.TB.
type exampleTB struct {
	testing.TB
}

func (*exampleTB) Helper() {}

func (*exampleTB) Fatalf(format string, args ...any) {
	panic(fmt.Sprintf("test failed: "+format, args...))
}
