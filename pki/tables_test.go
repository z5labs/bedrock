// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package pki_test

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/z5labs/bedrock/pki/pkitest"
)

// This file builds the tables the tests and the fuzz tests' seed corpora
// share: valid inputs, on which pki returns what the standard library
// returns, and lenient ones, which the standard library accepts, or of
// which it silently drops a part, and pki rejects.

// pemOf encodes der as a PEM block of type typ, with headers.
func pemOf(typ string, der []byte, headers map[string]string) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: typ, Headers: headers, Bytes: der})
}

// derOf returns the bytes of the one PEM block in data.
func derOf(tb testing.TB, data []byte) []byte {
	tb.Helper()
	b, rest := pem.Decode(data)
	require.NotNil(tb, b)
	require.Empty(tb, rest)
	return b.Bytes
}

// noEnd returns block, a PEM block, without its END line.
func noEnd(tb testing.TB, block []byte) []byte {
	tb.Helper()
	i := bytes.Index(block, []byte("-----END "))
	require.Positive(tb, i, "noEnd is given a PEM block with an END line: %q", block)
	return block[:i]
}

// noNewline returns block, a PEM block, without the newline after its END
// line, as a file written without a final newline is.
func noNewline(block []byte) []byte {
	return bytes.TrimSuffix(block, []byte("\n"))
}

// cut returns the first half of block, as a file cut off while being
// written or copied.
func cut(block []byte) []byte {
	return block[:len(block)/2]
}

// chainFor returns a chain for key's public key, a leaf issued by a new CA
// and then the CA, as PEM. pkitest issues only ECDSA keys, so this is how
// the tables get RSA and Ed25519 pairs.
func chainFor(tb testing.TB, key crypto.Signer) []byte {
	tb.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(tb, err)
	now := time.Now()
	caTmpl := &x509.Certificate{
		Subject:               pkix.Name{CommonName: "pki test CA " + rand.Text()},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	require.NoError(tb, err)
	ca, err := x509.ParseCertificate(caDER)
	require.NoError(tb, err)
	leafTmpl := &x509.Certificate{
		Subject:     pkix.Name{CommonName: "api.internal"},
		DNSNames:    []string{"api.internal"},
		NotBefore:   now.Add(-time.Hour),
		NotAfter:    now.Add(24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, ca, key.Public(), caKey)
	require.NoError(tb, err)
	return slices.Concat(pemOf("CERTIFICATE", leafDER, nil), pemOf("CERTIFICATE", caDER, nil))
}

// pkcs8 encodes key as a PKCS#8 PRIVATE KEY block.
func pkcs8(tb testing.TB, key crypto.Signer) []byte {
	tb.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(tb, err)
	return pemOf("PRIVATE KEY", der, nil)
}

// material is what the tables are built from.
type material struct {
	// ca, ca2 and ca3 are three CAs, and caPEM, ca2PEM and ca3PEM their
	// certificates as PEM.
	ca, ca2, ca3          *pkitest.CA
	caPEM, ca2PEM, ca3PEM []byte

	// certPEM and keyPEM are an ECDSA pair from pkitest, issued by ca:
	// leafPEM then caPEM, and a PKCS#8 key. ecKey is its key, and
	// otherKeyPEM is the key of a second pair ca issued.
	certPEM, keyPEM, leafPEM []byte
	ecKey                    *ecdsa.PrivateKey
	otherKeyPEM              []byte

	// rsaKey and edKey are keys pkitest does not issue, and rsaChain and
	// edChain their chains.
	rsaKey            *rsa.PrivateKey
	rsaChain, edChain []byte
	edKey             ed25519.PrivateKey
}

func newMaterial(tb testing.TB) *material {
	tb.Helper()
	m := &material{ca: pkitest.NewCA(tb), ca2: pkitest.NewCA(tb), ca3: pkitest.NewCA(tb)}
	m.caPEM, m.ca2PEM, m.ca3PEM = m.ca.PEM(), m.ca2.PEM(), m.ca3.PEM()

	cert := m.ca.Issue(tb, "api.internal")
	m.certPEM, m.keyPEM = cert.PEM()
	m.leafPEM = m.certPEM[:len(m.certPEM)-len(m.caPEM)]
	require.Equal(tb, m.caPEM, m.certPEM[len(m.leafPEM):], "pkitest's chain is the leaf then the CA")
	m.ecKey = cert.TLS().PrivateKey.(*ecdsa.PrivateKey)
	_, m.otherKeyPEM = m.ca.Issue(tb, "other.internal").PEM()

	var err error
	m.rsaKey, err = rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(tb, err)
	m.rsaChain = chainFor(tb, m.rsaKey)
	_, m.edKey, err = ed25519.GenerateKey(rand.Reader)
	require.NoError(tb, err)
	m.edChain = chainFor(tb, m.edKey)
	return m
}

// pairRow is an input to KeyPair and tls.X509KeyPair.
type pairRow struct {
	name            string
	certPEM, keyPEM []byte
}

// validPairs is the equivalence table for KeyPair: inputs on which it
// returns what tls.X509KeyPair returns.
func validPairs(tb testing.TB, m *material) []pairRow {
	tb.Helper()
	sec1, err := x509.MarshalECPrivateKey(m.ecKey)
	require.NoError(tb, err)
	both := slices.Concat(m.certPEM, m.keyPEM)
	withHeaders := slices.Concat(pemOf("CERTIFICATE", derOf(tb, m.leafPEM), map[string]string{"Comment": "the leaf"}), m.caPEM)
	return []pairRow{
		{name: "ECDSA, PKCS#8, from pkitest", certPEM: m.certPEM, keyPEM: m.keyPEM},
		{name: "ECDSA, SEC 1", certPEM: m.certPEM, keyPEM: pemOf("EC PRIVATE KEY", sec1, nil)},
		{name: "RSA, PKCS#1", certPEM: m.rsaChain, keyPEM: pemOf("RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(m.rsaKey), nil)},
		{name: "RSA, PKCS#8", certPEM: m.rsaChain, keyPEM: pkcs8(tb, m.rsaKey)},
		{name: "Ed25519, PKCS#8", certPEM: m.edChain, keyPEM: pkcs8(tb, m.edKey)},
		{name: "the leaf alone", certPEM: m.leafPEM, keyPEM: m.keyPEM},
		{name: "one file, chain then key, as both", certPEM: both, keyPEM: both},
		{name: "one file, key then chain, as both", certPEM: slices.Concat(m.keyPEM, m.certPEM), keyPEM: slices.Concat(m.keyPEM, m.certPEM)},
		{
			name: "text between blocks",
			certPEM: slices.Concat(
				[]byte("# the chain of api.internal\nsubject=CN = api.internal\n"), m.leafPEM,
				[]byte("\nsubject=CN = the CA\n"), m.caPEM,
				[]byte("# the end\n"),
			),
			keyPEM: slices.Concat([]byte("# the key of api.internal\n"), m.keyPEM, []byte("\n")),
		},
		{name: "CRLF line endings", certPEM: crlf(m.certPEM), keyPEM: crlf(m.keyPEM)},
		// X509KeyPair reads no headers, and KeyPair accepts what it
		// accepts, so a header is not one of the mistakes it fails on.
		{name: "a certificate with headers", certPEM: withHeaders, keyPEM: m.keyPEM},
	}
}

// crlf returns data with its line endings written as CRLF.
func crlf(data []byte) []byte {
	return bytes.ReplaceAll(data, []byte("\n"), []byte("\r\n"))
}

// bundleRow is an input to Pool and x509.CertPool.AppendCertsFromPEM.
type bundleRow struct {
	name string
	data []byte
}

// validBundles is the equivalence table for Certificates and Pool: inputs
// on which Pool returns a pool equal to AppendCertsFromPEM's.
func validBundles(tb testing.TB, m *material) []bundleRow {
	tb.Helper()
	return []bundleRow{
		{name: "one certificate", data: m.caPEM},
		{name: "three certificates", data: slices.Concat(m.caPEM, m.ca2PEM, m.ca3PEM)},
		{name: "the same certificate twice", data: slices.Concat(m.caPEM, m.caPEM)},
		{
			name: "a comment header and a name above each certificate, as Mozilla's bundle has",
			data: slices.Concat(
				[]byte("##\n## Bundle of CA Root Certificates\n##\n## Certificate data from Mozilla.\n##\n\n"),
				[]byte("pkitest CA one\n==============\n"), m.caPEM,
				[]byte("\npkitest CA two\n==============\n"), m.ca2PEM,
			),
		},
		{
			name: "subject= and issuer= lines, as openssl prints",
			data: slices.Concat(
				[]byte("subject=CN = pkitest CA one\nissuer=CN = pkitest CA one\n"), m.caPEM,
				[]byte("---\nsubject=CN = pkitest CA two\nissuer=CN = pkitest CA two\n"), m.ca2PEM,
				[]byte("---\n"),
			),
		},
		{name: "CRLF line endings", data: crlf(slices.Concat(m.caPEM, m.ca2PEM))},
		{name: "a chain from chainFor", data: m.rsaChain},
	}
}

// lenientPair is an input that tls.X509KeyPair accepts, dropping a part of
// it, and KeyPair rejects with an error that starts with wantErr.
type lenientPair struct {
	pairRow
	wantErr string
}

// lenientPairs is KeyPair's half of the strictness table.
func lenientPairs(tb testing.TB, m *material) []lenientPair {
	tb.Helper()
	pub, err := x509.MarshalPKIXPublicKey(&m.ecKey.PublicKey)
	require.NoError(tb, err)
	ecParams := pemOf("EC PARAMETERS", []byte{0x06, 0x08, 0x2a, 0x86, 0x48, 0xce, 0x3d, 0x03, 0x01, 0x07}, nil)
	corrupt := pemOf("CERTIFICATE", cut(derOf(tb, m.ca2PEM)), nil)
	row := func(name string, certPEM, keyPEM []byte, wantErr string) lenientPair {
		return lenientPair{pairRow: pairRow{name: name, certPEM: certPEM, keyPEM: keyPEM}, wantErr: wantErr}
	}
	return []lenientPair{
		row("a stray block type in certPEM", slices.Concat(m.leafPEM, pemOf("PUBLIC KEY", pub, nil), m.caPEM), m.keyPEM,
			`pki: certPEM: block 2: a "PUBLIC KEY" block, neither a CERTIFICATE nor a private key`),
		row("a stray block type in keyPEM, as openssl ecparam -genkey writes", m.certPEM, slices.Concat(ecParams, m.keyPEM),
			`pki: keyPEM: block 1: a "EC PARAMETERS" block, neither a CERTIFICATE nor a private key`),
		row("a corrupt intermediate", slices.Concat(m.leafPEM, corrupt, m.caPEM), m.keyPEM,
			"pki: certPEM: block 2: x509: "),
		row("a second key", m.certPEM, slices.Concat(m.keyPEM, m.otherKeyPEM),
			"pki: keyPEM: block 2: a second private key"),
		row("no END line in the middle of the chain", slices.Concat(m.leafPEM, noEnd(tb, m.ca2PEM), m.caPEM), m.keyPEM,
			"pki: certPEM: block 2: -----BEGIN starts no PEM block"),
		row("no END line at the end of the chain", slices.Concat(m.leafPEM, noEnd(tb, m.caPEM)), m.keyPEM,
			"pki: certPEM: block 2: -----BEGIN starts no PEM block"),
		row("no END line on a second key", m.certPEM, slices.Concat(m.keyPEM, noEnd(tb, m.otherKeyPEM)),
			"pki: keyPEM: block 2: -----BEGIN starts no PEM block"),
		row("two files joined without a newline", slices.Concat(m.leafPEM, noNewline(m.ca2PEM), m.caPEM), m.keyPEM,
			"pki: certPEM: block 2: -----BEGIN starts no PEM block"),
		row("a chain cut inside its second certificate", slices.Concat(m.leafPEM, cut(m.caPEM)), m.keyPEM,
			"pki: certPEM: block 2: -----BEGIN starts no PEM block"),
	}
}

// lenientBundle is an input that AppendCertsFromPEM accepts, dropping a
// part of it, and Certificates and Pool reject with an error that starts
// with wantErr.
type lenientBundle struct {
	bundleRow
	wantErr string
}

// lenientBundles is Certificates' and Pool's half of the strictness table.
func lenientBundles(tb testing.TB, m *material) []lenientBundle {
	tb.Helper()
	pub, err := x509.MarshalPKIXPublicKey(&m.ecKey.PublicKey)
	require.NoError(tb, err)
	withHeaders := pemOf("CERTIFICATE", derOf(tb, m.ca2PEM), map[string]string{"Comment": "pkitest CA two"})
	corrupt := pemOf("CERTIFICATE", cut(derOf(tb, m.ca2PEM)), nil)
	row := func(name string, data []byte, wantErr string) lenientBundle {
		return lenientBundle{bundleRow: bundleRow{name: name, data: data}, wantErr: wantErr}
	}
	return []lenientBundle{
		row("a stray block type", slices.Concat(m.caPEM, pemOf("PUBLIC KEY", pub, nil), m.ca3PEM),
			`pki: block 2: a "PUBLIC KEY" block, not a CERTIFICATE`),
		row("a private key in a bundle", slices.Concat(m.caPEM, m.keyPEM),
			`pki: block 2: a "PRIVATE KEY" block, not a CERTIFICATE`),
		row("a corrupt certificate", slices.Concat(m.caPEM, corrupt, m.ca3PEM),
			"pki: block 2: x509: "),
		row("no END line in the middle", slices.Concat(m.caPEM, noEnd(tb, m.ca2PEM), m.ca3PEM),
			"pki: block 2: -----BEGIN starts no PEM block"),
		row("no END line at the end", slices.Concat(m.caPEM, noEnd(tb, m.ca2PEM)),
			"pki: block 2: -----BEGIN starts no PEM block"),
		row("two files joined without a newline", slices.Concat(m.caPEM, noNewline(m.ca2PEM), m.ca3PEM),
			"pki: block 2: -----BEGIN starts no PEM block"),
		row("a certificate with headers", slices.Concat(m.caPEM, withHeaders, m.ca3PEM),
			"pki: block 2: a CERTIFICATE block with headers"),
		row("a bundle cut inside its second certificate", slices.Concat(m.caPEM, cut(m.ca2PEM)),
			"pki: block 2: -----BEGIN starts no PEM block"),
	}
}
