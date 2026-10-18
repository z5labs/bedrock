// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package hosttest

import (
	"context"
	"io"
	"net"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/require"
)

// The pipe Dial makes is not visible to a test once Dial has failed, so
// these tests hand connect a pipe of their own and check both of its ends.

func TestListener_connect_closes_both_ends_when_ctx_is_done(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := New(t)
		ln, err := h.Listen("tcp", "api:80")
		require.NoError(t, err)
		defer func() { require.NoError(t, ln.Close()) }()

		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		client, server := net.Pipe()
		err = ln.(*listener).connect(ctx, client, server)
		require.ErrorIs(t, err, context.Canceled)
		requireClosed(t, client, server)
	})
}

func TestListener_connect_closes_both_ends_when_the_listener_closes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := New(t)
		ln, err := h.Listen("tcp", "api:80")
		require.NoError(t, err)
		require.NoError(t, ln.Close())

		client, server := net.Pipe()
		err = ln.(*listener).connect(t.Context(), client, server)
		require.ErrorIs(t, err, errListenerClosed)
		requireClosed(t, client, server)
	})
}

func requireClosed(t *testing.T, conns ...net.Conn) {
	t.Helper()
	for _, c := range conns {
		_, err := c.Write([]byte("x"))
		require.ErrorIs(t, err, io.ErrClosedPipe)
	}
}
