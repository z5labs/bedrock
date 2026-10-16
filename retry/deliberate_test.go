// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package retry_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeliberateFailure(t *testing.T) {
	require.Fail(t, "deliberate failure to show CI gates retry")
}
