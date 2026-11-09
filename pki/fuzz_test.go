// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package pki_test

import (
	"crypto"
	"crypto/tls"
	"crypto/x509"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/z5labs/bedrock/pki"
)

// FuzzKeyPair checks that KeyPair accepts nothing X509KeyPair rejects, and
// that on every input it accepts it returns what X509KeyPair returns. Its
// seed corpus is the equivalence and strictness tables.
func FuzzKeyPair(f *testing.F) {
	m := newMaterial(f)
	for _, row := range validPairs(f, m) {
		f.Add(row.certPEM, row.keyPEM)
	}
	for _, row := range lenientPairs(f, m) {
		f.Add(row.certPEM, row.keyPEM)
	}

	f.Fuzz(func(t *testing.T, certPEM, keyPEM []byte) {
		got, err := pki.KeyPair(certPEM, keyPEM)
		if err != nil {
			require.Equal(t, tls.Certificate{}, got)
			return
		}
		want, err := tls.X509KeyPair(certPEM, keyPEM)
		require.NoError(t, err, "KeyPair accepted what X509KeyPair rejects")
		require.Equal(t, want, got)
	})
}

// FuzzPool checks that Pool and Certificates accept nothing that
// AppendCertsFromPEM rejects, and that on every input they accept they
// return what AppendCertsFromPEM keeps. Its seed corpus is the equivalence
// and strictness tables.
func FuzzPool(f *testing.F) {
	m := newMaterial(f)
	for _, row := range validBundles(f, m) {
		f.Add(row.data)
	}
	for _, row := range lenientBundles(f, m) {
		f.Add(row.data)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		certs, certsErr := pki.Certificates(data)
		pool, err := pki.Pool(data)
		require.Equal(t, certsErr, err, "Pool fails as Certificates does")
		if err != nil {
			require.Nil(t, certs)
			require.Nil(t, pool)
			return
		}
		want := x509.NewCertPool()
		require.True(t, want.AppendCertsFromPEM(data), "Pool accepted what AppendCertsFromPEM rejects")
		require.True(t, want.Equal(pool), "the pools are equal")
		require.Equal(t, stdCertificates(data), certs)
	})
}

// allowedNotImplemented are the NotImplementedErrors with which go-pkcs12
// rejects a structure that RFC 7292 allows and pki decodes: an
// AuthenticatedSafe of more than two ContentInfos, and SHA-384, which
// go-pkcs12 doesn't implement for a MAC, a PRF or PBMAC1.
var allowedNotImplemented = []string{
	"pkcs12: expected between 1 and 2 items in the authenticated safe",
	"pkcs12: MAC digest algorithm not supported: 2.16.840.1.101.3.4.2.2",
	"pkcs12: pbes2 prf 1.2.840.113549.2.10 is not supported",
	"pkcs12: PBMAC1 PRF 1.2.840.113549.2.10 is not supported",
	"pkcs12: PBMAC1 MAC algorithm 1.2.840.113549.2.10 is not supported",
}

// FuzzPKCS12KeyPair checks that PKCS12KeyPair accepts nothing go-pkcs12's
// DecodeChain rejects, unless DecodeChain rejects it for a structure
// RFC 7292 allows, or because it is BER, and that the two return the same
// pair whenever DecodeChain's first certificate matches the key. It checks
// too that a file it accepts and that file re-encoded as BER decode to the
// same pair. Its seed corpus is the PKCS#12 tables, their files re-encoded
// as BER at every level, and RFC 9579's test vectors.
func FuzzPKCS12KeyPair(f *testing.F) {
	m := newMaterial(f)
	for _, row := range validPKCS12Pairs(f, m) {
		f.Add(row.data, row.password)
		f.Add(berFile(f, row.data, row.password), row.password)
	}
	for _, row := range validTrustStores(f, m) {
		f.Add(row.data, row.password)
	}
	for _, row := range strictPKCS12(f, m) {
		f.Add(row.data, row.password)
	}
	for n := 1; n <= 6; n++ {
		f.Add(rfc9579(f, n), "1234")
	}

	f.Fuzz(func(t *testing.T, data []byte, password string) {
		got, err := pki.PKCS12KeyPair(data, password)
		if err != nil {
			require.Equal(t, tls.Certificate{}, got)
			return
		}
		if ber, ok := berOf(data); ok {
			gotBER, err := pki.PKCS12KeyPair(ber, password)
			require.NoError(t, err, "PKCS12KeyPair rejected the BER of what it accepts")
			require.Equal(t, got, gotBER)
		}
		want, stdErr := stdPair(data, password)
		if stdErr != nil {
			if _, ok := berOf(data); !ok {
				// go-pkcs12 decodes DER only.
				return
			}
			ni, ok := errorsAsNotImplemented(stdErr)
			require.True(t, ok, "PKCS12KeyPair accepted what DecodeChain rejects: %v", stdErr)
			allowed := false
			for _, prefix := range allowedNotImplemented {
				allowed = allowed || strings.HasPrefix(ni.Error(), prefix)
			}
			require.True(t, allowed, "PKCS12KeyPair accepted what DecodeChain doesn't implement: %v", ni)
			return
		}
		key, ok := want.PrivateKey.(crypto.Signer)
		require.True(t, ok)
		pub, ok := key.Public().(interface{ Equal(crypto.PublicKey) bool })
		require.True(t, ok)
		if pub.Equal(want.Leaf.PublicKey) {
			require.Equal(t, want, got)
		}
	})
}
