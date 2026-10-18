// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

//go:build conformance

package hosttest_test

import (
	"testing"

	"github.com/z5labs/bedrock/host"
)

// TestConformance_loopback runs the conformance scenarios on real loopback
// sockets, so it runs only with -tags conformance. It needs 127.0.0.2 on
// the loopback interface, as Linux has.
func TestConformance_loopback(t *testing.T) {
	testConformance(t, func(t *testing.T, scenario func(*testing.T, listenFunc, dialFunc)) {
		h := host.OS()
		scenario(t, h.Listen, h.Dial)
	})
}
