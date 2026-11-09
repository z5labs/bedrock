// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package pki_test

import (
	"crypto"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/asn1"
	"errors"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/require"
	sslmate "software.sslmate.com/src/go-pkcs12"

	"github.com/z5labs/bedrock/pki"
	"github.com/z5labs/bedrock/pki/internal/pkcs12"
	"github.com/z5labs/bedrock/pki/pkitest"
)

// stdPair is what go-pkcs12's DecodeChain returns for data, assembled into
// a tls.Certificate.
func stdPair(data []byte, password string) (tls.Certificate, error) {
	key, leaf, cas, err := sslmate.DecodeChain(data, password)
	if err != nil {
		return tls.Certificate{}, err
	}
	der := [][]byte{leaf.Raw}
	for _, ca := range cas {
		der = append(der, ca.Raw)
	}
	return tls.Certificate{Certificate: der, PrivateKey: key, Leaf: leaf}, nil
}

func TestPKCS12KeyPair_returns_what_DecodeChain_returns(t *testing.T) {
	for _, row := range validPKCS12Pairs(t, newMaterial(t)) {
		t.Run(row.name, func(t *testing.T) {
			want, err := stdPair(row.data, row.password)
			require.NoError(t, err)

			got, err := pki.PKCS12KeyPair(row.data, row.password)
			require.NoError(t, err)
			require.Equal(t, want, got)
		})
	}
}

func TestPKCS12Pool_returns_what_DecodeTrustStore_returns(t *testing.T) {
	for _, row := range validTrustStores(t, newMaterial(t)) {
		t.Run(row.name, func(t *testing.T) {
			certs, err := sslmate.DecodeTrustStore(row.data, row.password)
			require.NoError(t, err)
			require.Len(t, certs, 3)
			want := x509.NewCertPool()
			for _, cert := range certs {
				want.AddCert(cert)
			}

			got, err := pki.PKCS12Pool(row.data, row.password)
			require.NoError(t, err)
			require.True(t, want.Equal(got), "the pools are equal")
		})
	}
}

func TestPKCS12KeyPair_fails_on_RC2_naming_it(t *testing.T) {
	m := newMaterial(t)
	data := encode(t, sslmate.LegacyRC2, chains(t, m)[0], password)
	_, err := stdPair(data, password)
	require.NoError(t, err, "go-pkcs12 decodes RC2")

	got, err := pki.PKCS12KeyPair(data, password)
	require.EqualError(t, err, "pki: PKCS#12: AuthenticatedSafe: ContentInfo 1: encryptedData: unsupported encryption algorithm pbeWithSHAAnd40BitRC2-CBC (1.2.840.113549.1.12.1.6)")
	require.Equal(t, tls.Certificate{}, got)
}

func TestPKCS12KeyPair_decodes_RFC_9579s_test_vectors(t *testing.T) {
	for _, n := range []int{1, 2, 3} {
		t.Run("valid A."+string(rune('0'+n)), func(t *testing.T) {
			pair, err := pki.PKCS12KeyPair(rfc9579(t, n), "1234")
			require.NoError(t, err)
			require.Len(t, pair.Certificate, 1)
			require.Equal(t, "tt", pair.Leaf.Subject.CommonName)
		})
	}

	for _, row := range []struct {
		n       int
		name    string
		wantErr string
	}{
		{4, "an incorrect iteration count", "pki: PKCS#12: macData: the MAC doesn't verify with the password"},
		{5, "an incorrect salt", "pki: PKCS#12: macData: the MAC doesn't verify with the password"},
		{6, "a missing key length", "pki: PKCS#12: macData: PBMAC1: PBKDF2 has no key length"},
	} {
		t.Run("invalid A."+string(rune('0'+row.n))+", "+row.name, func(t *testing.T) {
			got, err := pki.PKCS12KeyPair(rfc9579(t, row.n), "1234")
			require.EqualError(t, err, row.wantErr)
			require.Equal(t, tls.Certificate{}, got)
		})
	}
}

func TestPKCS12_rejects_what_go_pkcs12_accepts_leniently(t *testing.T) {
	for _, row := range strictPKCS12(t, newMaterial(t)) {
		t.Run(row.name, func(t *testing.T) {
			if row.trustStore {
				_, stdErr := sslmate.DecodeTrustStore(row.data, row.password)
				require.Equal(t, row.stdAccepts, stdErr == nil, "DecodeTrustStore: %v", stdErr)

				got, err := pki.PKCS12Pool(row.data, row.password)
				require.EqualError(t, err, row.wantErr)
				require.Nil(t, got)
				return
			}
			_, stdErr := stdPair(row.data, row.password)
			require.Equal(t, row.stdAccepts, stdErr == nil, "DecodeChain: %v", stdErr)

			got, err := pki.PKCS12KeyPair(row.data, row.password)
			require.EqualError(t, err, row.wantErr)
			require.Equal(t, tls.Certificate{}, got)
		})
	}
}

func TestPKCS12KeyPair_finds_the_leaf_after_the_root(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ca := pkitest.NewCA(t)
		issued := ca.Issue(t, "api.internal").TLS()
		root, err := x509.ParseCertificate(issued.Certificate[1])
		require.NoError(t, err)
		// The root first, as Azure Key Vault writes it.
		data, err := sslmate.Modern2023.Encode(issued.PrivateKey, root, []*x509.Certificate{issued.Leaf}, password)
		require.NoError(t, err)

		std, err := stdPair(data, password)
		require.NoError(t, err)
		require.Equal(t, root, std.Leaf, "DecodeChain returns the root as the leaf")

		pair, err := pki.PKCS12KeyPair(data, password)
		require.NoError(t, err)
		requireSamePair(t, issued, pair, "the leaf comes first")

		_, cliState, srvErr, cliErr := handshake(t,
			&tls.Config{Certificates: []tls.Certificate{pair}},
			&tls.Config{ServerName: "api.internal", RootCAs: ca.Pool()},
		)
		require.NoError(t, cliErr)
		require.NoError(t, srvErr)
		require.True(t, cliState.HandshakeComplete)
	})
}

func TestPKCS12_fails_on_a_wrong_password_or_a_corrupt_MAC(t *testing.T) {
	m := newMaterial(t)
	pairData := encode(t, sslmate.Modern2023, chains(t, m)[0], password)
	stores := validTrustStores(t, m)
	storeData := stores[1].data

	corrupt := func(data []byte) []byte {
		var p pkcs12.PFX
		_, err := asn1.Unmarshal(data, &p)
		require.NoError(t, err)
		p.MacData.Mac.Digest[0] ^= 1
		return marshal(t, p)
	}

	for _, row := range []struct {
		name     string
		data     []byte
		password string
		pool     bool
	}{
		{"PKCS12KeyPair, a wrong password", pairData, "wrong", false},
		{"PKCS12KeyPair, the empty password", pairData, "", false},
		{"PKCS12KeyPair, a corrupt MAC", corrupt(pairData), password, false},
		{"PKCS12Pool, a wrong password", storeData, "wrong", true},
		{"PKCS12Pool, a corrupt MAC", corrupt(storeData), password, true},
	} {
		t.Run(row.name, func(t *testing.T) {
			var err error
			if row.pool {
				var got *x509.CertPool
				got, err = pki.PKCS12Pool(row.data, row.password)
				require.Nil(t, got)
			} else {
				var got tls.Certificate
				got, err = pki.PKCS12KeyPair(row.data, row.password)
				require.Equal(t, tls.Certificate{}, got)
			}
			require.ErrorIs(t, err, pki.ErrIncorrectPassword)
			require.ErrorIs(t, err, pkcs12.ErrMAC)
			require.EqualError(t, err, "pki: PKCS#12: macData: the MAC doesn't verify with the password")
		})
	}
}

func TestPKCS12KeyPair_fails_on_a_file_it_cannot_use(t *testing.T) {
	m := newMaterial(t)
	ec := chains(t, m)[0]
	leaf := ec.leaf().Raw
	x25519, err := ecdh.X25519().GenerateKey(rand.Reader)
	require.NoError(t, err)
	wrongType := safeBag(pkcs12.OIDCertBag, marshal(t, pkcs12.CertBag{
		ID:    asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 22, 2},
		Value: explicit(marshal(t, leaf)),
	}))
	null := marshal(t, asn1.NullRawValue)

	for _, row := range []struct {
		name    string
		bags    []pkcs12.SafeBag
		wantErr string
	}{
		{"a second key", []pkcs12.SafeBag{keyBag(t, m.ecKey), certBag(t, leaf), keyBag(t, m.rsaKey)},
			"pki: PKCS#12: bag 3: a second private key"},
		{"a keyBag that isn't one value", []pkcs12.SafeBag{safeBag(pkcs12.OIDKeyBag, append(null, null...)), certBag(t, leaf)},
			"pki: PKCS#12: bag 1: keyBag: 2 trailing bytes"},
		{"a keyBag that isn't PKCS#8", []pkcs12.SafeBag{safeBag(pkcs12.OIDKeyBag, null), certBag(t, leaf)},
			"pki: PKCS#12: bag 1: keyBag: asn1: "},
		{"a pkcs8ShroudedKeyBag that isn't one", []pkcs12.SafeBag{certBag(t, leaf), safeBag(pkcs12.OIDPKCS8ShroudedKeyBag, null)},
			"pki: PKCS#12: bag 2: pkcs8ShroudedKeyBag: asn1: "},
		{"an X25519 key", []pkcs12.SafeBag{keyBag(t, x25519), certBag(t, leaf)},
			"pki: PKCS#12: bag 1: keyBag: a *ecdh.PrivateKey, which crypto/tls can't sign with"},
		{"a certificate of another type", []pkcs12.SafeBag{keyBag(t, m.ecKey), wrongType},
			"pki: PKCS#12: bag 2: certBag: a certificate of type sdsiCertificate (1.2.840.113549.1.9.22.2), not x509Certificate"},
		{"a certificate that doesn't parse", []pkcs12.SafeBag{keyBag(t, m.ecKey), certBag(t, leaf[:len(leaf)-1])},
			"pki: PKCS#12: bag 2: certBag: x509: "},
		{"no key", []pkcs12.SafeBag{certBag(t, leaf)}, "pki: PKCS#12: no private key"},
		{"no certificate", []pkcs12.SafeBag{keyBag(t, m.ecKey)}, "pki: PKCS#12: no certificate"},
		{"nothing", nil, "pki: PKCS#12: no private key"},
	} {
		t.Run(row.name, func(t *testing.T) {
			got, err := pki.PKCS12KeyPair(noMAC(t, row.bags), "")
			require.Error(t, err)
			require.True(t, strings.HasPrefix(err.Error(), row.wantErr), "the error %q starts with %q", err, row.wantErr)
			require.Equal(t, tls.Certificate{}, got)
		})
	}

	t.Run("a file that isn't PKCS#12", func(t *testing.T) {
		got, err := pki.PKCS12KeyPair(m.certPEM, "")
		require.ErrorContains(t, err, "pki: PKCS#12: PFX: asn1: ")
		require.NotErrorIs(t, err, pki.ErrIncorrectPassword)
		require.Equal(t, tls.Certificate{}, got)
	})
}

func TestPKCS12Pool_fails_on_a_file_it_cannot_use(t *testing.T) {
	m := newMaterial(t)
	ca := certsOf(t, m.caPEM)[0].Raw

	for _, row := range []struct {
		name    string
		bags    []pkcs12.SafeBag
		wantErr string
	}{
		{"a certificate that doesn't parse", []pkcs12.SafeBag{certBag(t, ca, trusted(t)), certBag(t, ca[:len(ca)-1], trusted(t))},
			"pki: PKCS#12: bag 2: certBag: x509: "},
		{"no certificate", []pkcs12.SafeBag{}, "pki: PKCS#12: no certificate"},
	} {
		t.Run(row.name, func(t *testing.T) {
			got, err := pki.PKCS12Pool(noMAC(t, row.bags), "")
			require.Error(t, err)
			require.True(t, strings.HasPrefix(err.Error(), row.wantErr), "the error %q starts with %q", err, row.wantErr)
			require.Nil(t, got)
		})
	}

	t.Run("a file that isn't PKCS#12", func(t *testing.T) {
		got, err := pki.PKCS12Pool(m.caPEM, "")
		require.ErrorContains(t, err, "pki: PKCS#12: PFX: asn1: ")
		require.Nil(t, got)
	})
}

func TestPKCS12KeyPair_decodes_pkitest_for_a_TLS_handshake(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ca := pkitest.NewCA(t)
		issued := ca.Issue(t, "api.internal").TLS()
		root, err := x509.ParseCertificate(issued.Certificate[1])
		require.NoError(t, err)
		data, err := sslmate.Modern2023.Encode(issued.PrivateKey, issued.Leaf, []*x509.Certificate{root}, password)
		require.NoError(t, err)

		pair, err := pki.PKCS12KeyPair(data, password)
		require.NoError(t, err)
		requireSamePair(t, issued, pair)

		_, cliState, srvErr, cliErr := handshake(t,
			&tls.Config{Certificates: []tls.Certificate{pair}},
			&tls.Config{ServerName: "api.internal", RootCAs: ca.Pool()},
		)
		require.NoError(t, cliErr)
		require.NoError(t, srvErr)
		require.True(t, cliState.HandshakeComplete)
		require.Equal(t, pair.Leaf.Raw, cliState.PeerCertificates[0].Raw, "the client verified the decoded certificate")
	})
}

// requireSamePair checks that got holds want's chain, leaf and key.
func requireSamePair(t *testing.T, want, got tls.Certificate, msgAndArgs ...any) {
	t.Helper()
	require.Equal(t, want.Certificate, got.Certificate, msgAndArgs...)
	require.Equal(t, want.Leaf.Raw, got.Leaf.Raw, msgAndArgs...)
	key, ok := want.PrivateKey.(interface{ Equal(crypto.PrivateKey) bool })
	require.True(t, ok)
	require.True(t, key.Equal(got.PrivateKey), msgAndArgs...)
}

// errorsAsNotImplemented reports whether err is go-pkcs12's
// NotImplementedError.
func errorsAsNotImplemented(err error) (sslmate.NotImplementedError, bool) {
	var ni sslmate.NotImplementedError
	ok := errors.As(err, &ni)
	return ni, ok
}
