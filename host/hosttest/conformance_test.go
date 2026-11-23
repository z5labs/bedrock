// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package hosttest_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"io"
	"math/big"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/z5labs/bedrock/host/hosttest"
)

// dialFunc is the shape of a DialTCP or a DialUDP, which the helpers that
// dial take.
type dialFunc func(ctx context.Context, network, address string) (net.Conn, error)

// testConformance runs the scenarios that the in-memory network must get
// right exactly as a real one does. TestConformance_in_memory runs them on
// a Host's four network methods, each inside a synctest bubble, and
// TestConformance_loopback, behind the conformance build tag, runs them on
// real loopback sockets through host.IPNetwork and host.OS.
//
// run runs one scenario on the network to test, which it makes for that
// scenario. The TCP scenarios call its ListenTCP and DialTCP.
//
// The two names are 127.0.0.1 and 127.0.0.2, both on the loopback
// interface, and every first listener takes port 0, so the scenarios work
// on a machine whose ports are already in use. The TLS handshakes serve on
// every name a certificate for 127.0.0.1 alone: a client dialling
// 127.0.0.1 verifies it, and one dialling 127.0.0.2 rejects it.
//
// The TLS close rows hold a connection to a socket's buffers. A client
// that rejects a certificate writes its alert while the server may still
// be writing the rest of its first flight, and a TLS Close writes
// close_notify to a peer that may never read it. On a socket both writes
// land in a buffer and return.
func testConformance(t *testing.T, run func(t *testing.T, scenario func(t *testing.T, n ipNetwork))) {
	t.Run("two names on one port both serve", func(t *testing.T) {
		run(t, func(t *testing.T, n ipNetwork) {
			one := conformanceListen(t, n, "127.0.0.1:0")
			port := strconv.Itoa(portOf(t, one.Addr()))
			two := conformanceListen(t, n, net.JoinHostPort("127.0.0.2", port))
			serve(t, one, "one")
			serve(t, two, "two")

			require.Equal(t, "one", dialRead(t, n.DialTCP, net.JoinHostPort("127.0.0.1", port)))
			require.Equal(t, "two", dialRead(t, n.DialTCP, net.JoinHostPort("127.0.0.2", port)))
		})
	})

	t.Run("EADDRINUSE for a name on a port every name holds", func(t *testing.T) {
		run(t, func(t *testing.T, n ipNetwork) {
			every := conformanceListen(t, n, ":0")
			defer func() { require.NoError(t, every.Close()) }()

			_, err := n.ListenTCP(t.Context(), "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(portOf(t, every.Addr()))))
			require.ErrorIs(t, err, syscall.EADDRINUSE)
		})
	})

	t.Run("EADDRINUSE for every name on a port a name holds", func(t *testing.T) {
		run(t, func(t *testing.T, n ipNetwork) {
			one := conformanceListen(t, n, "127.0.0.1:0")
			defer func() { require.NoError(t, one.Close()) }()

			_, err := n.ListenTCP(t.Context(), "tcp", ":"+strconv.Itoa(portOf(t, one.Addr())))
			require.ErrorIs(t, err, syscall.EADDRINUSE)
		})
	})

	t.Run("ECONNREFUSED for a name no listener holds", func(t *testing.T) {
		run(t, func(t *testing.T, n ipNetwork) {
			one := conformanceListen(t, n, "127.0.0.1:0")
			serve(t, one, "one")

			_, err := n.DialTCP(t.Context(), "tcp", net.JoinHostPort("127.0.0.2", strconv.Itoa(portOf(t, one.Addr()))))
			require.ErrorIs(t, err, syscall.ECONNREFUSED)
		})
	})

	t.Run("ECONNREFUSED after the listener closes", func(t *testing.T) {
		run(t, func(t *testing.T, n ipNetwork) {
			one := conformanceListen(t, n, "127.0.0.1:0")
			address := one.Addr().String()
			require.NoError(t, one.Close())

			_, err := n.DialTCP(t.Context(), "tcp", address)
			require.ErrorIs(t, err, syscall.ECONNREFUSED)
		})
	})

	t.Run("TLS handshake", func(t *testing.T) {
		run(t, func(t *testing.T, n ipNetwork) {
			ca := newTestCA(t)
			every := conformanceListen(t, n, ":0")
			port := strconv.Itoa(portOf(t, every.Addr()))
			serveTLS(t, every, ca.serverConfig(t, "127.0.0.1"), "hello")

			require.Equal(t, "hello", dialTLS(t, n.DialTCP, ca.pool, net.JoinHostPort("127.0.0.1", port)))
		})
	})

	t.Run("TLS handshake rejects a name the certificate doesn't cover", func(t *testing.T) {
		run(t, func(t *testing.T, n ipNetwork) {
			ca := newTestCA(t)
			every := conformanceListen(t, n, ":0")
			port := strconv.Itoa(portOf(t, every.Addr()))
			serveTLS(t, every, ca.serverConfig(t, "127.0.0.1"), "hello")

			c, err := n.DialTCP(t.Context(), "tcp", net.JoinHostPort("127.0.0.2", port))
			require.NoError(t, err)
			defer func() { _ = c.Close() }()
			tc := tls.Client(c, &tls.Config{RootCAs: ca.pool, ServerName: "127.0.0.2", MinVersion: tls.VersionTLS12})

			// A handshake that stalls fails with ctx's error instead.
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			err = tc.HandshakeContext(ctx)
			_, ok := errors.AsType[x509.HostnameError](err)
			require.True(t, ok, "the handshake failed with %v, not a hostname error", err)
		})
	})

	t.Run("TLS close while the peer doesn't read", func(t *testing.T) {
		run(t, func(t *testing.T, n ipNetwork) {
			took, ok := closeNotifyStandoff(t, n, false)
			require.True(t, ok, "Close hasn't returned after %v", took)
			require.Less(t, took, 5*time.Second, "Close waited out crypto/tls's write deadline")
		})
	})

	t.Run("TLS close whose write deadline is reset while close_notify is written", func(t *testing.T) {
		run(t, func(t *testing.T, n ipNetwork) {
			took, ok := closeNotifyStandoff(t, n, true)
			require.True(t, ok, "Close hasn't returned after %v", took)
		})
	})
}

// closeNotifyStandoff serves TLS with a server that never reads after its
// handshake, as kfake's reader doesn't while a response is unwritten. It
// closes the client's connection and, with reset, resets the connection's
// write deadline once crypto/tls is writing close_notify, as franz-go does
// after a write on the same connection. It returns how long Close took,
// or false if it hadn't returned after 10s.
func closeNotifyStandoff(t *testing.T, n ipNetwork, reset bool) (time.Duration, bool) {
	t.Helper()
	ca := newTestCA(t)
	config := ca.serverConfig(t, "127.0.0.1")
	config.SessionTicketsDisabled = true
	ln := conformanceListen(t, n, "127.0.0.1:0")
	defer func() { _ = ln.Close() }()

	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	served := make(chan struct{})
	go func() {
		defer close(served)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		srv := tls.Server(conn, config)
		if err := srv.Handshake(); err != nil {
			return
		}
		<-release
	}()
	defer func() {
		unblock()
		_ = ln.Close()
		<-served
	}()

	conn, err := n.DialTCP(t.Context(), "tcp", ln.Addr().String())
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	w := &alertConn{Conn: conn, alert: make(chan struct{})}
	client := tls.Client(w, &tls.Config{RootCAs: ca.pool, ServerName: "127.0.0.1", MinVersion: tls.VersionTLS12})
	require.NoError(t, client.HandshakeContext(t.Context()))

	w.closing.Store(true)
	start := time.Now()
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		_ = client.Close()
	}()
	<-w.alert
	if reset {
		// franz-go discards this error too. Close may already have
		// finished, and the reset fails on a closed connection.
		_ = client.SetWriteDeadline(time.Time{})
	}

	select {
	case <-closed:
		return time.Since(start), true
	case <-time.After(10 * time.Second):
		unblock()
		<-closed
		return time.Since(start), false
	}
}

// alertConn is the connection beneath crypto/tls. Once closing is set, it
// closes alert as its first write, which is close_notify, begins.
type alertConn struct {
	net.Conn
	alert   chan struct{}
	closing atomic.Bool
	alerted atomic.Bool
}

func (c *alertConn) Write(b []byte) (int, error) {
	if c.closing.Load() && c.alerted.CompareAndSwap(false, true) {
		close(c.alert)
	}
	return c.Conn.Write(b)
}

func conformanceListen(t *testing.T, n ipNetwork, address string) net.Listener {
	t.Helper()
	ln, err := n.ListenTCP(t.Context(), "tcp", address)
	require.NoError(t, err)
	return ln
}

// TestConformance_in_memory runs every conformance table on a Host's four
// network methods, each scenario inside a synctest bubble of its own.
func TestConformance_in_memory(t *testing.T) {
	inMemory := func(t *testing.T, scenario func(*testing.T, ipNetwork)) {
		synctest.Test(t, func(t *testing.T) {
			scenario(t, hosttest.New(t))
		})
	}
	testConformance(t, inMemory)
	t.Run("udp", func(t *testing.T) {
		testConformanceUDP(t, inMemory)
	})
	t.Run("networks", func(t *testing.T) {
		testConformanceNetworks(t, inMemory)
	})
}

// testCA is a certificate authority made for one test. It signs server
// certificates for the names a test dials.
type testCA struct {
	cert  *x509.Certificate
	key   *ecdsa.PrivateKey
	pool  *x509.CertPool
	valid time.Time
}

// newTestCA makes a self-signed CA. Its certificates are valid for an hour
// either side of now, which inside a synctest bubble is the bubble's time.
func newTestCA(t *testing.T) *testCA {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "hosttest CA"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)

	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return &testCA{cert: cert, key: key, pool: pool, valid: now}
}

// serverConfig returns a server's tls.Config holding a certificate the CA
// signed for name, an IP address or a DNS name.
func (ca *testCA) serverConfig(t *testing.T, name string) *tls.Config {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: name},
		NotBefore:    ca.valid.Add(-time.Hour),
		NotAfter:     ca.valid.Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	if ip := net.ParseIP(name); ip != nil {
		tmpl.IPAddresses = []net.IP{ip}
	} else {
		tmpl.DNSNames = []string{name}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, &key.PublicKey, ca.key)
	require.NoError(t, err)

	return &tls.Config{
		Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}},
		MinVersion:   tls.VersionTLS12,
	}
}

// serveTLS accepts on ln until it is closed, completing a TLS handshake on
// every connection, writing msg and closing it. The listener is closed,
// and every connection's goroutine waited for, when the test ends.
func serveTLS(t *testing.T, ln net.Listener, config *tls.Config, msg string) {
	t.Helper()
	var wg sync.WaitGroup
	wg.Go(func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			wg.Go(func() {
				tc := tls.Server(c, config)
				if tc.Handshake() == nil {
					_, _ = io.WriteString(tc, msg)
				}
				_ = tc.Close()
			})
		}
	})
	t.Cleanup(func() {
		_ = ln.Close()
		wg.Wait()
	})
}

// dialTLS dials address, completes a TLS handshake trusting only roots and
// verifying the host of address, and returns everything the server writes.
func dialTLS(t *testing.T, dial dialFunc, roots *x509.CertPool, address string) string {
	t.Helper()
	name, _, err := net.SplitHostPort(address)
	require.NoError(t, err)
	c, err := dial(t.Context(), "tcp", address)
	require.NoError(t, err)

	tc := tls.Client(c, &tls.Config{RootCAs: roots, ServerName: name, MinVersion: tls.VersionTLS12})
	// The server closes its end once it has written, so the close_notify
	// this Close sends may find no one to read it. The connection is
	// closed either way.
	defer func() { _ = tc.Close() }()
	require.NoError(t, tc.HandshakeContext(t.Context()))
	b, err := io.ReadAll(tc)
	require.NoError(t, err)
	return string(b)
}
