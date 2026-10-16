package fit_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Deliberately failing, to show that the Build check fails on a failing test
// in fit. Reverted in the next commit.
func TestDeliberateFailure(t *testing.T) {
	require.Fail(t, "deliberate failure to show the Build check fails on fit")
}
