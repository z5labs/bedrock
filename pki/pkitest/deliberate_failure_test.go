// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package pkitest_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// A deliberately failing test, to show it failing the Build check.
// Reverted in the next commit.
func TestDeliberateFailure(t *testing.T) {
	require.Fail(t, "deliberate failure in pki")
}
