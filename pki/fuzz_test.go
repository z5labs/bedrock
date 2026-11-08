// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package pki_test

import (
	"crypto/tls"
	"crypto/x509"
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
