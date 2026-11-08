// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package pkitest_test

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/z5labs/bedrock/pki/pkitest"
)

// chain returns the parsed leaf and CA certificates of c.
func chain(t *testing.T, c *pkitest.Certificate) (leaf, ca *x509.Certificate) {
	t.Helper()
	tc := c.TLS()
	require.Len(t, tc.Certificate, 2)
	leaf, err := x509.ParseCertificate(tc.Certificate[0])
	require.NoError(t, err)
	ca, err = x509.ParseCertificate(tc.Certificate[1])
	require.NoError(t, err)
	return leaf, ca
}

// requireSerial asserts that cert's serial number is positive and at most
// 20 octets, as RFC 5280 requires.
func requireSerial(t *testing.T, cert *x509.Certificate) {
	t.Helper()
	require.Positive(t, cert.SerialNumber.Sign(), "the serial number is positive")
	require.LessOrEqual(t, len(cert.SerialNumber.Bytes()), 20, "the serial number is at most 20 octets")
}

// blocks decodes every PEM block in data, and requires that nothing but
// blocks is in it.
func blocks(t *testing.T, data []byte) []*pem.Block {
	t.Helper()
	var bs []*pem.Block
	for len(data) > 0 {
		b, rest := pem.Decode(data)
		require.NotNil(t, b, "only PEM blocks: %q", data)
		bs = append(bs, b)
		data = rest
	}
	return bs
}

func TestNewCA_generates_a_new_CA_each_call(t *testing.T) {
	_, one := chain(t, pkitest.NewCA(t).Issue(t, "api.internal"))
	_, two := chain(t, pkitest.NewCA(t).Issue(t, "api.internal"))

	require.True(t, one.IsCA)
	require.True(t, two.IsCA)
	require.NotEqual(t, one.SerialNumber, two.SerialNumber, "each CA has its own serial number")
	require.False(t, one.PublicKey.(*ecdsa.PublicKey).Equal(two.PublicKey), "each CA has its own key")
	require.NotEqual(t, one.Subject.String(), two.Subject.String(), "each CA has its own subject")
	require.Equal(t, one.Subject.String(), one.Issuer.String(), "a CA is self-signed")
	require.NoError(t, one.CheckSignatureFrom(one), "a CA is self-signed")
	requireSerial(t, one)
	requireSerial(t, two)
}

func TestCA_Issue_generates_a_new_certificate_each_call(t *testing.T) {
	ca := pkitest.NewCA(t)
	one, _ := chain(t, ca.Issue(t, "api.internal"))
	two, _ := chain(t, ca.Issue(t, "api.internal"))

	require.NotEqual(t, one.SerialNumber, two.SerialNumber, "each certificate has its own serial number")
	require.False(t, one.PublicKey.(*ecdsa.PublicKey).Equal(two.PublicKey), "each certificate has its own key")
	requireSerial(t, one)
	requireSerial(t, two)
}

func TestCA_Issue_puts_each_name_in_its_SAN_field(t *testing.T) {
	ca := pkitest.NewCA(t)
	leaf, _ := chain(t, ca.Issue(t, "api.internal", "127.0.0.1", "*.svc.internal", "::1"))

	require.Equal(t, "api.internal", leaf.Subject.CommonName)
	require.Equal(t, []string{"api.internal", "*.svc.internal"}, leaf.DNSNames)
	require.Len(t, leaf.IPAddresses, 2)
	require.True(t, leaf.IPAddresses[0].Equal(net.ParseIP("127.0.0.1")))
	require.True(t, leaf.IPAddresses[1].Equal(net.ParseIP("::1")))

	for _, name := range []string{"api.internal", "a.svc.internal", "127.0.0.1", "::1"} {
		require.NoError(t, leaf.VerifyHostname(name), name)
	}
	require.Error(t, leaf.VerifyHostname("other.internal"))
}

func TestCA_Issue_with_no_names_is_a_client_certificate(t *testing.T) {
	ca := pkitest.NewCA(t)
	leaf, _ := chain(t, ca.Issue(t))

	require.Empty(t, leaf.Subject.CommonName)
	require.Empty(t, leaf.DNSNames)
	require.Empty(t, leaf.IPAddresses)

	_, err := leaf.Verify(x509.VerifyOptions{
		Roots:     ca.Pool(),
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	})
	require.NoError(t, err, "a certificate with no names verifies for client authentication")
}

func TestCA_Pool(t *testing.T) {
	ca := pkitest.NewCA(t)
	other := pkitest.NewCA(t)
	leaf, _ := chain(t, ca.Issue(t, "api.internal"))

	_, err := leaf.Verify(x509.VerifyOptions{DNSName: "api.internal", Roots: ca.Pool()})
	require.NoError(t, err, "a certificate verifies against its own CA's Pool")

	_, err = leaf.Verify(x509.VerifyOptions{DNSName: "api.internal", Roots: other.Pool()})
	_, ok := errors.AsType[x509.UnknownAuthorityError](err)
	require.True(t, ok, "a certificate fails verification against another CA's Pool: %v", err)
}

func TestCA_PEM_is_one_CERTIFICATE_block_of_the_Pool(t *testing.T) {
	ca := pkitest.NewCA(t)
	_, root := chain(t, ca.Issue(t, "api.internal"))

	data := ca.PEM()
	bs := blocks(t, data)
	require.Len(t, bs, 1)
	require.Equal(t, "CERTIFICATE", bs[0].Type)
	require.Empty(t, bs[0].Headers)
	require.Equal(t, root.Raw, bs[0].Bytes, "the block is the CA's certificate")
	require.True(t, bytes.HasSuffix(data, []byte("\n")), "the block ends with a newline")

	pool := x509.NewCertPool()
	require.True(t, pool.AppendCertsFromPEM(data))
	require.True(t, pool.Equal(ca.Pool()), "the PEM's pool is the CA's Pool")
	require.False(t, pool.Equal(pkitest.NewCA(t).Pool()))
}

func TestCertificate_TLS_carries_the_leaf_then_the_CA(t *testing.T) {
	ca := pkitest.NewCA(t)
	c := ca.Issue(t, "api.internal")
	tc := c.TLS()
	leaf, root := chain(t, c)

	require.False(t, leaf.IsCA)
	require.Equal(t, []string{"api.internal"}, leaf.DNSNames)
	require.True(t, root.IsCA)
	require.NoError(t, leaf.CheckSignatureFrom(root), "the second certificate is the CA that signed the first")
	require.Equal(t, leaf.Raw, tc.Leaf.Raw)

	key, ok := tc.PrivateKey.(*ecdsa.PrivateKey)
	require.True(t, ok)
	require.True(t, key.PublicKey.Equal(leaf.PublicKey), "the private key is the leaf's")
}

func TestCertificate_PEM(t *testing.T) {
	ca := pkitest.NewCA(t)
	c := ca.Issue(t, "api.internal")
	tc := c.TLS()
	certPEM, keyPEM := c.PEM()

	t.Run("certPEM is the leaf then the CA", func(t *testing.T) {
		bs := blocks(t, certPEM)
		require.Len(t, bs, 2)
		for i, b := range bs {
			require.Equal(t, "CERTIFICATE", b.Type)
			require.Empty(t, b.Headers)
			require.Equal(t, tc.Certificate[i], b.Bytes)
		}
		require.True(t, bytes.HasSuffix(certPEM, ca.PEM()), "the second block is the CA's PEM")
	})

	t.Run("keyPEM is one PKCS#8 PRIVATE KEY block", func(t *testing.T) {
		bs := blocks(t, keyPEM)
		require.Len(t, bs, 1)
		require.Equal(t, "PRIVATE KEY", bs[0].Type)
		require.Empty(t, bs[0].Headers)
		key, err := x509.ParsePKCS8PrivateKey(bs[0].Bytes)
		require.NoError(t, err)
		require.True(t, tc.PrivateKey.(*ecdsa.PrivateKey).Equal(key), "the block is the certificate's key")
	})

	t.Run("every block ends with a newline", func(t *testing.T) {
		require.True(t, bytes.HasSuffix(certPEM, []byte("\n")))
		require.True(t, bytes.HasSuffix(keyPEM, []byte("\n")))
	})

	requirePair := func(t *testing.T, pair tls.Certificate) {
		t.Helper()
		require.Equal(t, tc.Certificate, pair.Certificate, "the pair's chain is TLS's")
		require.True(t, tc.PrivateKey.(*ecdsa.PrivateKey).Equal(pair.PrivateKey), "the pair's key is TLS's")
	}

	t.Run("tls.X509KeyPair decodes it", func(t *testing.T) {
		pair, err := tls.X509KeyPair(c.PEM())
		require.NoError(t, err)
		requirePair(t, pair)
	})

	t.Run("tls.X509KeyPair decodes it as one file", func(t *testing.T) {
		file := slices.Concat(certPEM, keyPEM)
		pair, err := tls.X509KeyPair(file, file)
		require.NoError(t, err)
		requirePair(t, pair)
	})

	t.Run("each call returns the same PEM", func(t *testing.T) {
		again, againKey := c.PEM()
		require.Equal(t, certPEM, again)
		require.Equal(t, keyPEM, againKey)
	})
}

// handshake runs a TLS handshake over net.Pipe between a server with
// server and a client with client, and returns each side's connection
// state and error. It must run inside a synctest bubble.
func handshake(t *testing.T, server, client *tls.Config) (srvState, cliState tls.ConnectionState, srvErr, cliErr error) {
	t.Helper()
	srvConn, cliConn := net.Pipe()
	srv := tls.Server(srvConn, server)
	cli := tls.Client(cliConn, client)

	// Each side closes its end when its handshake is done, so that a side
	// still writing to the other sees the pipe closed rather than blocking.
	var srvCloseErr error
	var wg sync.WaitGroup
	wg.Go(func() {
		srvErr = srv.HandshakeContext(t.Context())
		srvState = srv.ConnectionState()
		srvCloseErr = srvConn.Close()
	})
	cliErr = cli.HandshakeContext(t.Context())
	cliState = cli.ConnectionState()
	require.NoError(t, cliConn.Close())
	wg.Wait()
	require.NoError(t, srvCloseErr)
	return srvState, cliState, srvErr, cliErr
}

func TestCA_TLS_handshake_over_a_pipe(t *testing.T) {
	t.Run("server only", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ca := pkitest.NewCA(t)
			cert := ca.Issue(t, "api.internal")

			_, cliState, srvErr, cliErr := handshake(t,
				&tls.Config{Certificates: []tls.Certificate{cert.TLS()}},
				&tls.Config{ServerName: "api.internal", RootCAs: ca.Pool()},
			)
			require.NoError(t, cliErr)
			require.NoError(t, srvErr)
			require.True(t, cliState.HandshakeComplete)
			require.Equal(t, cert.TLS().Certificate[0], cliState.PeerCertificates[0].Raw, "the client verified the issued certificate")
		})
	})

	t.Run("mutual", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ca := pkitest.NewCA(t)
			serverCert := ca.Issue(t, "api.internal")
			clientCert := ca.Issue(t)

			srvState, _, srvErr, cliErr := handshake(t,
				&tls.Config{
					Certificates: []tls.Certificate{serverCert.TLS()},
					ClientAuth:   tls.RequireAndVerifyClientCert,
					ClientCAs:    ca.Pool(),
				},
				&tls.Config{
					ServerName:   "api.internal",
					RootCAs:      ca.Pool(),
					Certificates: []tls.Certificate{clientCert.TLS()},
				},
			)
			require.NoError(t, cliErr)
			require.NoError(t, srvErr)
			require.True(t, srvState.HandshakeComplete)
			require.Len(t, srvState.VerifiedChains, 1)
			require.Equal(t, clientCert.TLS().Certificate[0], srvState.PeerCertificates[0].Raw, "the server verified the client's certificate")
		})
	})

	t.Run("a client rejects a name the certificate does not hold", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ca := pkitest.NewCA(t)

			_, _, srvErr, cliErr := handshake(t,
				&tls.Config{Certificates: []tls.Certificate{ca.Issue(t, "api.internal").TLS()}},
				&tls.Config{ServerName: "other.internal", RootCAs: ca.Pool()},
			)
			_, ok := errors.AsType[x509.HostnameError](cliErr)
			require.True(t, ok, "the certificate does not cover the name: %v", cliErr)
			require.Error(t, srvErr, "the server sees the client abort")
		})
	})

	t.Run("a client rejects a certificate from a second CA", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ca := pkitest.NewCA(t)
			other := pkitest.NewCA(t)

			_, _, srvErr, cliErr := handshake(t,
				&tls.Config{Certificates: []tls.Certificate{ca.Issue(t, "api.internal").TLS()}},
				&tls.Config{ServerName: "api.internal", RootCAs: other.Pool()},
			)
			_, ok := errors.AsType[x509.UnknownAuthorityError](cliErr)
			require.True(t, ok, "a client trusting another CA rejects the certificate: %v", cliErr)
			require.Error(t, srvErr, "the server sees the client abort")
		})
	})
}

// TestNewCA_validity_follows_the_bubble_clock builds a CA inside a bubble,
// where time.Now() is the bubble's clock, not the wall clock.
func TestNewCA_validity_follows_the_bubble_clock(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		now := time.Now()
		ca := pkitest.NewCA(t)
		cert := ca.Issue(t, "api.internal")
		leaf, root := chain(t, cert)

		for _, c := range []*x509.Certificate{leaf, root} {
			require.True(t, now.Add(-time.Hour).Equal(c.NotBefore), "valid from an hour before: %v", c.NotBefore)
			require.True(t, now.Add(24*time.Hour).Equal(c.NotAfter), "valid until a day after: %v", c.NotAfter)
		}

		server := &tls.Config{Certificates: []tls.Certificate{cert.TLS()}}
		client := &tls.Config{ServerName: "api.internal", RootCAs: ca.Pool()}

		_, _, srvErr, cliErr := handshake(t, server, client)
		require.NoError(t, cliErr, "the certificate verifies on the bubble's clock")
		require.NoError(t, srvErr)

		// A day and an hour later on the bubble's clock, past NotAfter.
		time.Sleep(25 * time.Hour)
		_, _, srvErr, cliErr = handshake(t, server, client)
		invalid, ok := errors.AsType[x509.CertificateInvalidError](cliErr)
		require.True(t, ok, "%v", cliErr)
		require.Equal(t, x509.Expired, invalid.Reason, "a day later on the bubble's clock, the certificate has expired")
		require.Error(t, srvErr, "the server sees the client abort")
	})
}

// fatalTB records the failure reported through Fatalf and stops the
// goroutine, as testing.T does.
type fatalTB struct {
	testing.TB
	mu    sync.Mutex
	fatal string
}

func (*fatalTB) Helper() {}

func (tb *fatalTB) Fatalf(format string, args ...any) {
	tb.mu.Lock()
	tb.fatal = fmt.Sprintf(format, args...)
	tb.mu.Unlock()
	runtime.Goexit()
}

func TestCA_Issue_fails_the_test_through_tb(t *testing.T) {
	ca := pkitest.NewCA(t)
	tb := &fatalTB{TB: t}

	var got *pkitest.Certificate
	var wg sync.WaitGroup
	wg.Go(func() {
		got = ca.Issue(tb, "bücher.example")
	})
	wg.Wait()

	require.Nil(t, got)
	require.True(t, strings.HasPrefix(tb.fatal, `pkitest: Issue(["bücher.example"]): `), "the failure names what was asked for: %s", tb.fatal)
}
