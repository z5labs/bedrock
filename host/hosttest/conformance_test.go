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
	"io"
	"math/big"
	"net"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/z5labs/bedrock/host/hosttest"
)

// listenFunc and dialFunc are the network under test, the Listen and Dial
// of a host.Host.
type (
	listenFunc func(network, address string) (net.Listener, error)
	dialFunc   func(ctx context.Context, network, address string) (net.Conn, error)
)

// testConformance runs the scenarios that the in-memory network must get
// right exactly as a real one does. TestConformance_in_memory runs them on
// a Host, each inside a synctest bubble, and TestConformance_loopback,
// behind the conformance build tag, runs them on real loopback sockets
// through host.OS.
//
// run runs one scenario on the Listen and Dial to test, which it makes
// for that scenario.
//
// The two names are 127.0.0.1 and 127.0.0.2, both on the loopback
// interface, and every first listener takes port 0, so the scenarios work
// on a machine whose ports are already in use. The TLS handshake serves on
// every name a certificate for 127.0.0.1, which the client dials and
// verifies.
//
// The TLS row does not dial a name the certificate does not cover. A
// client that rejects the certificate writes its alert while the server is
// still writing the rest of its first flight, and a net.Pipe, unlike a
// socket, has no buffer to take either write, so the two would block each
// other.
func testConformance(t *testing.T, run func(t *testing.T, scenario func(t *testing.T, listen listenFunc, dial dialFunc))) {
	t.Run("two names on one port both serve", func(t *testing.T) {
		run(t, func(t *testing.T, listen listenFunc, dial dialFunc) {
			one := conformanceListen(t, listen, "127.0.0.1:0")
			port := strconv.Itoa(portOf(t, one.Addr()))
			two := conformanceListen(t, listen, net.JoinHostPort("127.0.0.2", port))
			serve(t, one, "one")
			serve(t, two, "two")

			require.Equal(t, "one", dialRead(t, dial, net.JoinHostPort("127.0.0.1", port)))
			require.Equal(t, "two", dialRead(t, dial, net.JoinHostPort("127.0.0.2", port)))
		})
	})

	t.Run("EADDRINUSE for a name on a port every name holds", func(t *testing.T) {
		run(t, func(t *testing.T, listen listenFunc, _ dialFunc) {
			every := conformanceListen(t, listen, ":0")
			defer func() { require.NoError(t, every.Close()) }()

			_, err := listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(portOf(t, every.Addr()))))
			require.ErrorIs(t, err, syscall.EADDRINUSE)
		})
	})

	t.Run("EADDRINUSE for every name on a port a name holds", func(t *testing.T) {
		run(t, func(t *testing.T, listen listenFunc, _ dialFunc) {
			one := conformanceListen(t, listen, "127.0.0.1:0")
			defer func() { require.NoError(t, one.Close()) }()

			_, err := listen("tcp", ":"+strconv.Itoa(portOf(t, one.Addr())))
			require.ErrorIs(t, err, syscall.EADDRINUSE)
		})
	})

	t.Run("ECONNREFUSED for a name no listener holds", func(t *testing.T) {
		run(t, func(t *testing.T, listen listenFunc, dial dialFunc) {
			one := conformanceListen(t, listen, "127.0.0.1:0")
			serve(t, one, "one")

			_, err := dial(t.Context(), "tcp", net.JoinHostPort("127.0.0.2", strconv.Itoa(portOf(t, one.Addr()))))
			require.ErrorIs(t, err, syscall.ECONNREFUSED)
		})
	})

	t.Run("ECONNREFUSED after the listener closes", func(t *testing.T) {
		run(t, func(t *testing.T, listen listenFunc, dial dialFunc) {
			one := conformanceListen(t, listen, "127.0.0.1:0")
			address := one.Addr().String()
			require.NoError(t, one.Close())

			_, err := dial(t.Context(), "tcp", address)
			require.ErrorIs(t, err, syscall.ECONNREFUSED)
		})
	})

	t.Run("TLS handshake", func(t *testing.T) {
		run(t, func(t *testing.T, listen listenFunc, dial dialFunc) {
			ca := newTestCA(t)
			every := conformanceListen(t, listen, ":0")
			port := strconv.Itoa(portOf(t, every.Addr()))
			serveTLS(t, every, ca.serverConfig(t, "127.0.0.1"), "hello")

			require.Equal(t, "hello", dialTLS(t, dial, ca.pool, net.JoinHostPort("127.0.0.1", port)))
		})
	})
}

func conformanceListen(t *testing.T, listen listenFunc, address string) net.Listener {
	t.Helper()
	ln, err := listen("tcp", address)
	require.NoError(t, err)
	return ln
}

func TestConformance_in_memory(t *testing.T) {
	testConformance(t, func(t *testing.T, scenario func(*testing.T, listenFunc, dialFunc)) {
		synctest.Test(t, func(t *testing.T) {
			h := hosttest.New(t)
			scenario(t, h.Listen, h.Dial)
		})
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
