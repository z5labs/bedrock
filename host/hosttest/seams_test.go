// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package hosttest_test

import (
	"context"
	"errors"
	"net"
	"syscall"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/require"

	"github.com/z5labs/bedrock/host/hosttest"
)

// errFromNetwork is what every method of recordingNetwork returns, and
// errFromOn what every On func below returns, so a test can tell which of
// the two a Seams method called.
var (
	errFromNetwork = errors.New("from Network")
	errFromOn      = errors.New("from an On func")
)

// methodKey is the ctx key under which seamsMethods names the method it
// calls, so a test can tell the ctx reached the func that took it.
type methodKey struct{}

// call is one call a Seams method made: the method it reached and what it
// was passed.
type call struct {
	method           string
	ctx              context.Context
	network, address string
}

// recordingNetwork is a Seams' Network that records every call and fails
// it with errFromNetwork.
type recordingNetwork struct {
	calls []call
}

func (n *recordingNetwork) ListenTCP(ctx context.Context, network, address string) (net.Listener, error) {
	n.calls = append(n.calls, call{"ListenTCP", ctx, network, address})
	return nil, errFromNetwork
}

func (n *recordingNetwork) DialTCP(ctx context.Context, network, address string) (net.Conn, error) {
	n.calls = append(n.calls, call{"DialTCP", ctx, network, address})
	return nil, errFromNetwork
}

func (n *recordingNetwork) ListenUDP(ctx context.Context, network, address string) (net.PacketConn, error) {
	n.calls = append(n.calls, call{"ListenUDP", ctx, network, address})
	return nil, errFromNetwork
}

func (n *recordingNetwork) DialUDP(ctx context.Context, network, address string) (net.Conn, error) {
	n.calls = append(n.calls, call{"DialUDP", ctx, network, address})
	return nil, errFromNetwork
}

// seamsMethods calls each of the four methods of s once, in a fixed order,
// with a ctx and arguments that name the method, and returns each one's
// error by method.
func seamsMethods(ctx context.Context, s hosttest.Seams) map[string]error {
	errs := make(map[string]error)
	_, errs["ListenTCP"] = s.ListenTCP(context.WithValue(ctx, methodKey{}, "ListenTCP"), "tcp", "ListenTCP:1")
	_, errs["DialTCP"] = s.DialTCP(context.WithValue(ctx, methodKey{}, "DialTCP"), "tcp", "DialTCP:2")
	_, errs["ListenUDP"] = s.ListenUDP(context.WithValue(ctx, methodKey{}, "ListenUDP"), "udp", "ListenUDP:3")
	_, errs["DialUDP"] = s.DialUDP(context.WithValue(ctx, methodKey{}, "DialUDP"), "udp", "DialUDP:4")
	return errs
}

// args is the network and address seamsMethods passes to each method.
var args = map[string][2]string{
	"ListenTCP": {"tcp", "ListenTCP:1"},
	"DialTCP":   {"tcp", "DialTCP:2"},
	"ListenUDP": {"udp", "ListenUDP:3"},
	"DialUDP":   {"udp", "DialUDP:4"},
}

var methods = []string{"ListenTCP", "DialTCP", "ListenUDP", "DialUDP"}

func TestSeams_calls_Network_for_every_method_with_no_On_func(t *testing.T) {
	n := new(recordingNetwork)
	errs := seamsMethods(t.Context(), hosttest.Seams{Network: n})

	require.Len(t, n.calls, 4)
	for i, method := range methods {
		require.ErrorIs(t, errs[method], errFromNetwork, method)
		c := n.calls[i]
		require.Equal(t, method, c.method)
		require.Equal(t, method, c.ctx.Value(methodKey{}), "%s passes its ctx through", method)
		require.Equal(t, args[method], [2]string{c.network, c.address}, "%s passes its arguments through", method)
	}
}

func TestSeams_an_On_func_replaces_only_its_own_method(t *testing.T) {
	for _, replaced := range methods {
		t.Run(replaced, func(t *testing.T) {
			var got []call
			record := func(ctx context.Context, network, address string) {
				got = append(got, call{replaced, ctx, network, address})
			}
			s := hosttest.Seams{Network: new(recordingNetwork)}
			switch replaced {
			case "ListenTCP":
				s.OnListenTCP = func(ctx context.Context, network, address string) (net.Listener, error) {
					record(ctx, network, address)
					return nil, errFromOn
				}
			case "DialTCP":
				s.OnDialTCP = func(ctx context.Context, network, address string) (net.Conn, error) {
					record(ctx, network, address)
					return nil, errFromOn
				}
			case "ListenUDP":
				s.OnListenUDP = func(ctx context.Context, network, address string) (net.PacketConn, error) {
					record(ctx, network, address)
					return nil, errFromOn
				}
			case "DialUDP":
				s.OnDialUDP = func(ctx context.Context, network, address string) (net.Conn, error) {
					record(ctx, network, address)
					return nil, errFromOn
				}
			}
			errs := seamsMethods(t.Context(), s)

			require.Len(t, got, 1)
			require.Equal(t, replaced, got[0].ctx.Value(methodKey{}), "the On func gets the method's ctx")
			require.Equal(t, args[replaced], [2]string{got[0].network, got[0].address}, "the On func gets the method's arguments")

			var others []string
			for _, method := range methods {
				if method == replaced {
					require.ErrorIs(t, errs[method], errFromOn, method)
					continue
				}
				require.ErrorIs(t, errs[method], errFromNetwork, method)
				others = append(others, method)
			}
			var called []string
			for _, c := range s.Network.(*recordingNetwork).calls {
				called = append(called, c.method)
			}
			require.Equal(t, others, called, "the other three call Network")
		})
	}
}

func TestSeams_panics_naming_Seams_Network_when_a_method_falls_back_to_a_nil_Network(t *testing.T) {
	var s hosttest.Seams
	testCases := map[string]func(){
		"ListenTCP": func() { _, _ = s.ListenTCP(t.Context(), "tcp", "api:80") },
		"DialTCP":   func() { _, _ = s.DialTCP(t.Context(), "tcp", "api:80") },
		"ListenUDP": func() { _, _ = s.ListenUDP(t.Context(), "udp", "dns:53") },
		"DialUDP":   func() { _, _ = s.DialUDP(t.Context(), "udp", "dns:53") },
	}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			require.PanicsWithValue(t,
				"hosttest: Seams."+method+" has no On"+method+" and Seams.Network is nil; Seams.Network is required",
				testCases[method])
		})
	}
}

// TestSeams_starts_no_goroutines runs every method of a Seams over a Host,
// with and without an On func, inside a bubble, and leaves what they made
// open. synctest.Test fails with a deadlock if Seams left a goroutine
// blocked behind.
func TestSeams_starts_no_goroutines(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		th := hosttest.New(t)
		s := hosttest.Seams{Network: th}

		_, err := s.ListenTCP(t.Context(), "tcp", "api:80")
		require.NoError(t, err)
		_, err = s.DialTCP(t.Context(), "tcp", "api:81")
		require.ErrorIs(t, err, syscall.ECONNREFUSED)
		_, err = s.ListenUDP(t.Context(), "udp", "dns:53")
		require.NoError(t, err)
		c, err := s.DialUDP(t.Context(), "udp", "dns:53")
		require.NoError(t, err)
		_, err = c.Write([]byte("ping"))
		require.NoError(t, err)

		s.OnDialTCP = func(context.Context, string, string) (net.Conn, error) { return nil, errFromOn }
		_, err = s.DialTCP(t.Context(), "tcp", "api:80")
		require.ErrorIs(t, err, errFromOn)
	})
}
