package bedrock

import "github.com/stretchr/testify/require"

// Deliberately imports a third-party module from library code, to show that
// lint fails on it. Reverted in the next commit.
var _ = require.True
