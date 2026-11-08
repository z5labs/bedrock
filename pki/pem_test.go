// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package pki_test

import (
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"errors"
	"net"
	"slices"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/require"

	"github.com/z5labs/bedrock/pki"
	"github.com/z5labs/bedrock/pki/pkitest"
)

// stdCertificates decodes every CERTIFICATE block pem.Decode finds in data
// and every one x509.ParseCertificate parses, skipping the rest, as
// AppendCertsFromPEM does.
func stdCertificates(data []byte) []*x509.Certificate {
	var certs []*x509.Certificate
	for {
		b, rest := pem.Decode(data)
		if b == nil {
			return certs
		}
		data = rest
		if b.Type != "CERTIFICATE" || len(b.Headers) > 0 {
			continue
		}
		cert, err := x509.ParseCertificate(b.Bytes)
		if err != nil {
			continue
		}
		certs = append(certs, cert)
	}
}

func TestKeyPair_returns_what_X509KeyPair_returns(t *testing.T) {
	for _, row := range validPairs(t, newMaterial(t)) {
		t.Run(row.name, func(t *testing.T) {
			want, err := tls.X509KeyPair(row.certPEM, row.keyPEM)
			require.NoError(t, err)

			got, err := pki.KeyPair(row.certPEM, row.keyPEM)
			require.NoError(t, err)
			require.Equal(t, want, got)
			require.NotNil(t, got.Leaf)
			require.Equal(t, got.Certificate[0], got.Leaf.Raw, "Leaf is the first certificate of the chain")
		})
	}
}

func TestPool_returns_what_AppendCertsFromPEM_returns(t *testing.T) {
	for _, row := range validBundles(t, newMaterial(t)) {
		t.Run(row.name, func(t *testing.T) {
			want := x509.NewCertPool()
			require.True(t, want.AppendCertsFromPEM(row.data))

			got, err := pki.Pool(row.data)
			require.NoError(t, err)
			require.True(t, want.Equal(got), "the pools are equal")
		})
	}
}

func TestCertificates_decodes_every_certificate_in_order(t *testing.T) {
	for _, row := range validBundles(t, newMaterial(t)) {
		t.Run(row.name, func(t *testing.T) {
			got, err := pki.Certificates(row.data)
			require.NoError(t, err)
			require.Equal(t, stdCertificates(row.data), got)
		})
	}
}

func TestKeyPair_rejects_what_X509KeyPair_accepts_leniently(t *testing.T) {
	for _, row := range lenientPairs(t, newMaterial(t)) {
		t.Run(row.name, func(t *testing.T) {
			_, err := tls.X509KeyPair(row.certPEM, row.keyPEM)
			require.NoError(t, err, "X509KeyPair accepts it")

			got, err := pki.KeyPair(row.certPEM, row.keyPEM)
			require.ErrorContains(t, err, row.wantErr)
			require.Equal(t, row.wantErr, err.Error()[:len(row.wantErr)], "the error names the input and the block")
			require.Equal(t, tls.Certificate{}, got)
		})
	}
}

func TestPool_rejects_what_AppendCertsFromPEM_accepts_leniently(t *testing.T) {
	for _, row := range lenientBundles(t, newMaterial(t)) {
		t.Run(row.name, func(t *testing.T) {
			std := x509.NewCertPool()
			require.True(t, std.AppendCertsFromPEM(row.data), "AppendCertsFromPEM accepts it")

			pool, err := pki.Pool(row.data)
			require.ErrorContains(t, err, row.wantErr)
			require.Equal(t, row.wantErr, err.Error()[:len(row.wantErr)], "the error names the block")
			require.Nil(t, pool)

			certs, err := pki.Certificates(row.data)
			require.ErrorContains(t, err, row.wantErr)
			require.Nil(t, certs)
		})
	}
}

func TestCertificates_wraps_the_parse_error(t *testing.T) {
	m := newMaterial(t)
	der := cut(derOf(t, m.ca2PEM))
	_, parseErr := x509.ParseCertificate(der)
	require.Error(t, parseErr)

	_, err := pki.Certificates(slices.Concat(m.caPEM, pemOf("CERTIFICATE", der, nil)))
	require.EqualError(t, err, "pki: block 2: "+parseErr.Error())
	require.EqualError(t, errors.Unwrap(errors.Unwrap(err)), parseErr.Error(), "the parse error is wrapped")

	_, err = pki.KeyPair(slices.Concat(m.leafPEM, pemOf("CERTIFICATE", der, nil)), m.keyPEM)
	require.EqualError(t, err, "pki: certPEM: block 2: "+parseErr.Error())
}

func TestCertificates_fails_on_data_with_no_block(t *testing.T) {
	for name, data := range map[string][]byte{
		"nil":                      nil,
		"empty":                    {},
		"text":                     []byte("# no certificates here\n"),
		"an END line and no BEGIN": []byte("-----END CERTIFICATE-----\n"),
	} {
		t.Run(name, func(t *testing.T) {
			certs, err := pki.Certificates(data)
			require.EqualError(t, err, "pki: no PEM block")
			require.Nil(t, certs)

			pool, err := pki.Pool(data)
			require.EqualError(t, err, "pki: no PEM block")
			require.Nil(t, pool)
		})
	}
}

func TestCertificates_names_the_first_block_that_fails(t *testing.T) {
	m := newMaterial(t)
	_, err := pki.Certificates(slices.Concat(m.keyPEM, noEnd(t, m.caPEM)))
	require.ErrorContains(t, err, `pki: block 1: a "PRIVATE KEY" block`)

	_, err = pki.Certificates(noNewline(m.caPEM))
	require.NoError(t, err, "a file without a final newline is a whole block")

	_, err = pki.Certificates(slices.Concat(noNewline(m.caPEM), m.ca2PEM))
	require.ErrorContains(t, err, "pki: block 1: -----BEGIN starts no PEM block", "joining two files hides the first block too")
}

func TestKeyPair_fails_as_X509KeyPair_does(t *testing.T) {
	m := newMaterial(t)
	encrypted, err := asn1.Marshal(struct {
		Algorithm pkix.AlgorithmIdentifier
		Data      []byte
	}{
		// PBES2, from PKCS#5, as openssl pkcs8 -topk8 writes.
		Algorithm: pkix.AlgorithmIdentifier{Algorithm: asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 5, 13}},
		Data:      []byte(rand.Text()),
	})
	require.NoError(t, err)

	for _, row := range []pairRow{
		{name: "a key that does not match the leaf", certPEM: m.certPEM, keyPEM: m.otherKeyPEM},
		{name: "an RSA key under an ECDSA leaf", certPEM: m.certPEM, keyPEM: pkcs8(t, m.rsaKey)},
		{name: "an ENCRYPTED PRIVATE KEY block", certPEM: m.certPEM, keyPEM: pemOf("ENCRYPTED PRIVATE KEY", encrypted, nil)},
		{name: "no certificate", certPEM: m.keyPEM, keyPEM: m.keyPEM},
		{name: "no key", certPEM: m.certPEM, keyPEM: m.certPEM},
		{name: "nothing", certPEM: nil, keyPEM: nil},
	} {
		t.Run(row.name, func(t *testing.T) {
			_, stdErr := tls.X509KeyPair(row.certPEM, row.keyPEM)
			require.Error(t, stdErr)

			got, err := pki.KeyPair(row.certPEM, row.keyPEM)
			require.EqualError(t, err, "pki: "+stdErr.Error())
			require.EqualError(t, errors.Unwrap(err), stdErr.Error(), "X509KeyPair's error is wrapped")
			require.Equal(t, tls.Certificate{}, got)
		})
	}
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

func TestKeyPair_and_Pool_decode_pkitest_for_a_TLS_handshake(t *testing.T) {
	t.Run("server only", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ca := pkitest.NewCA(t)
			pair, err := pki.KeyPair(ca.Issue(t, "api.internal").PEM())
			require.NoError(t, err)
			roots, err := pki.Pool(ca.PEM())
			require.NoError(t, err)
			require.True(t, ca.Pool().Equal(roots))

			_, cliState, srvErr, cliErr := handshake(t,
				&tls.Config{Certificates: []tls.Certificate{pair}},
				&tls.Config{ServerName: "api.internal", RootCAs: roots},
			)
			require.NoError(t, cliErr)
			require.NoError(t, srvErr)
			require.True(t, cliState.HandshakeComplete)
			require.Equal(t, pair.Leaf.Raw, cliState.PeerCertificates[0].Raw, "the client verified the decoded certificate")
		})
	})

	t.Run("mutual", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ca := pkitest.NewCA(t)
			server, err := pki.KeyPair(ca.Issue(t, "api.internal").PEM())
			require.NoError(t, err)
			client, err := pki.KeyPair(ca.Issue(t).PEM())
			require.NoError(t, err)
			pool, err := pki.Pool(ca.PEM())
			require.NoError(t, err)

			srvState, _, srvErr, cliErr := handshake(t,
				&tls.Config{
					Certificates: []tls.Certificate{server},
					ClientAuth:   tls.RequireAndVerifyClientCert,
					ClientCAs:    pool,
				},
				&tls.Config{
					ServerName:   "api.internal",
					RootCAs:      pool,
					Certificates: []tls.Certificate{client},
				},
			)
			require.NoError(t, cliErr)
			require.NoError(t, srvErr)
			require.True(t, srvState.HandshakeComplete)
			require.Len(t, srvState.VerifiedChains, 1)
			require.Equal(t, client.Leaf.Raw, srvState.PeerCertificates[0].Raw, "the server verified the client's decoded certificate")
		})
	})
}
