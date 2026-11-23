// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package hosttest_test

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/require"
	"golang.org/x/net/dns/dnsmessage"

	"github.com/z5labs/bedrock/host/hosttest"
)

// TestHost_DialUDP_serves_Go_resolver resolves a name through Go's own
// resolver, dialling the Host. The resolver frames a query as a datagram
// only on a conn that is a net.PacketConn, and as a TCP stream, with a
// 2-byte length prefix, on any other, so the DNS server here takes a query
// only when it parses as a datagram with nothing before it.
func TestHost_DialUDP_serves_Go_resolver(t *testing.T) {
	const name = "service.hosttest.test."
	want := netip.MustParseAddr("192.0.2.7")

	synctest.Test(t, func(t *testing.T) {
		h := hosttest.New(t)
		server, err := h.ListenUDP(t.Context(), "udp", "dns:53")
		require.NoError(t, err)
		queries := make(chan dnsmessage.Question, 1)
		served := make(chan error, 1)
		go func() { served <- serveDNS(server, want, queries) }()

		r := &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
				if network == "udp" {
					return h.DialUDP(ctx, network, "dns:53")
				}
				return h.DialTCP(ctx, network, "dns:53")
			},
		}
		got, err := r.LookupNetIP(t.Context(), "ip4", name)
		require.NoError(t, err)
		require.Equal(t, []netip.Addr{want}, got)

		q := <-queries
		require.Equal(t, name, q.Name.String())
		require.Equal(t, dnsmessage.TypeA, q.Type)

		require.NoError(t, server.Close())
		require.ErrorIs(t, <-served, net.ErrClosed)
	})
}

// serveDNS answers every A query that pc takes with addr, and sends its
// question to queries, until pc fails to read, whose error it returns. It
// drops a datagram that doesn't parse as a DNS query, so a query framed
// for a stream, behind a length prefix, is never answered.
func serveDNS(pc net.PacketConn, addr netip.Addr, queries chan<- dnsmessage.Question) error {
	b := make([]byte, 512)
	for {
		n, from, err := pc.ReadFrom(b)
		if err != nil {
			return err
		}
		var msg dnsmessage.Message
		if err := msg.Unpack(b[:n]); err != nil || msg.Response || len(msg.Questions) != 1 {
			continue
		}
		q := msg.Questions[0]
		select {
		case queries <- q:
		default:
		}

		reply := dnsmessage.Message{
			Header: dnsmessage.Header{
				ID:                 msg.ID,
				Response:           true,
				Authoritative:      true,
				RecursionDesired:   msg.RecursionDesired,
				RecursionAvailable: true,
			},
			Questions: msg.Questions,
		}
		if q.Type == dnsmessage.TypeA {
			reply.Answers = []dnsmessage.Resource{{
				Header: dnsmessage.ResourceHeader{Name: q.Name, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET, TTL: 60},
				Body:   &dnsmessage.AResource{A: addr.As4()},
			}}
		}
		packed, err := reply.Pack()
		if err != nil {
			return errors.Join(errors.New("packing the reply"), err)
		}
		if _, err := pc.WriteTo(packed, from); err != nil {
			return err
		}
	}
}
