// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package circuit_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// DELIBERATE: a failing test, which the Build check must report.
func TestDeliberateFailure(t *testing.T) {
	require.Fail(t, "deliberate failure to show the Build check gates circuit")
}
