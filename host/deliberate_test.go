// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package host_test

import "testing"

func TestDeliberateFailure(t *testing.T) {
	t.Fatal("deliberate failure to show that CI gates host")
}
