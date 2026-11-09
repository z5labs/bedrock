// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package pkitest_test

import (
	"crypto/ecdsa"
	"crypto/sha1"
	"crypto/tls"
	"crypto/x509/pkix"
	"encoding/asn1"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/require"
	sslmate "software.sslmate.com/src/go-pkcs12"

	"github.com/z5labs/bedrock/pki"
	"github.com/z5labs/bedrock/pki/internal/pkcs12"
	"github.com/z5labs/bedrock/pki/pkitest"
)

// TestCertificate_PKCS12 is humustest's TestCertificate_PKCS12, decoding
// with pki instead of go-pkcs12.
func TestCertificate_PKCS12(t *testing.T) {
	ca := pkitest.NewCA(t)
	c := ca.Issue(t, "api.internal")
	tc := c.TLS()

	bundle, password := c.PKCS12(t)
	require.NotEmpty(t, password)

	pair, err := pki.PKCS12KeyPair(bundle, password)
	require.NoError(t, err)
	require.True(t, tc.PrivateKey.(*ecdsa.PrivateKey).Equal(pair.PrivateKey), "the bundle holds the certificate's key")
	require.Len(t, pair.Certificate, 2)
	require.Equal(t, tc.Certificate[0], pair.Certificate[0], "the bundle holds the leaf")
	require.Equal(t, tc.Certificate[1], pair.Certificate[1], "the bundle holds the CA")

	_, err = pki.PKCS12KeyPair(bundle, password+"x")
	require.ErrorIs(t, err, pki.ErrIncorrectPassword)

	_, err = pki.PKCS12KeyPair(bundle, "")
	require.ErrorIs(t, err, pki.ErrIncorrectPassword)

	_, again := c.PKCS12(t)
	require.NotEqual(t, password, again, "each call makes a new password")
}

func TestCertificate_PKCS12_round_trips_through_pki(t *testing.T) {
	c := pkitest.NewCA(t).Issue(t, "api.internal")
	tc := c.TLS()

	pair, err := pki.PKCS12KeyPair(c.PKCS12(t))
	require.NoError(t, err)
	require.Equal(t, tc.Certificate, pair.Certificate, "the chain is TLS's")
	require.True(t, tc.PrivateKey.(*ecdsa.PrivateKey).Equal(pair.PrivateKey), "the key is TLS's")
	require.Equal(t, tc.Leaf.Raw, pair.Leaf.Raw)
}

func TestCA_PKCS12_round_trips_through_pki(t *testing.T) {
	ca := pkitest.NewCA(t)

	pool, err := pki.PKCS12Pool(ca.PKCS12(t))
	require.NoError(t, err)
	require.True(t, pool.Equal(ca.Pool()), "the trust store's pool is the CA's Pool")
	require.False(t, pool.Equal(pkitest.NewCA(t).Pool()))

	bundle, password := ca.PKCS12(t)
	_, err = pki.PKCS12Pool(bundle, password+"x")
	require.ErrorIs(t, err, pki.ErrIncorrectPassword)
}

func TestPKCS12_handshake_over_a_pipe(t *testing.T) {
	t.Run("server only", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ca := pkitest.NewCA(t)
			pair, err := pki.PKCS12KeyPair(ca.Issue(t, "api.internal").PKCS12(t))
			require.NoError(t, err)
			pool, err := pki.PKCS12Pool(ca.PKCS12(t))
			require.NoError(t, err)

			_, cliState, srvErr, cliErr := handshake(t,
				&tls.Config{Certificates: []tls.Certificate{pair}},
				&tls.Config{ServerName: "api.internal", RootCAs: pool},
			)
			require.NoError(t, cliErr)
			require.NoError(t, srvErr)
			require.True(t, cliState.HandshakeComplete)
			require.Equal(t, pair.Certificate[0], cliState.PeerCertificates[0].Raw)
		})
	})

	t.Run("mutual", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ca := pkitest.NewCA(t)
			serverPair, err := pki.PKCS12KeyPair(ca.Issue(t, "api.internal").PKCS12(t))
			require.NoError(t, err)
			clientPair, err := pki.PKCS12KeyPair(ca.Issue(t).PKCS12(t))
			require.NoError(t, err)
			pool, err := pki.PKCS12Pool(ca.PKCS12(t))
			require.NoError(t, err)

			srvState, _, srvErr, cliErr := handshake(t,
				&tls.Config{
					Certificates: []tls.Certificate{serverPair},
					ClientAuth:   tls.RequireAndVerifyClientCert,
					ClientCAs:    pool,
				},
				&tls.Config{
					ServerName:   "api.internal",
					RootCAs:      pool,
					Certificates: []tls.Certificate{clientPair},
				},
			)
			require.NoError(t, cliErr)
			require.NoError(t, srvErr)
			require.True(t, srvState.HandshakeComplete)
			require.Len(t, srvState.VerifiedChains, 1)
			require.Equal(t, clientPair.Certificate[0], srvState.PeerCertificates[0].Raw)
		})
	})
}

func TestPKCS12_decodes_with_go_pkcs12(t *testing.T) {
	ca := pkitest.NewCA(t)
	c := ca.Issue(t, "api.internal")
	tc := c.TLS()

	t.Run("DecodeChain", func(t *testing.T) {
		key, leaf, cas, err := sslmate.DecodeChain(c.PKCS12(t))
		require.NoError(t, err)
		require.True(t, tc.PrivateKey.(*ecdsa.PrivateKey).Equal(key))
		require.Equal(t, tc.Certificate[0], leaf.Raw)
		require.Len(t, cas, 1)
		require.Equal(t, tc.Certificate[1], cas[0].Raw)
	})

	t.Run("DecodeTrustStore", func(t *testing.T) {
		certs, err := sslmate.DecodeTrustStore(ca.PKCS12(t))
		require.NoError(t, err)
		require.Len(t, certs, 1)
		require.Equal(t, tc.Certificate[1], certs[0].Raw)
	})
}

func TestPKCS12_makes_a_new_password_and_file_each_call(t *testing.T) {
	ca := pkitest.NewCA(t)
	c := ca.Issue(t, "api.internal")

	for name, encode := range map[string]func(testing.TB) ([]byte, string){
		"Certificate": c.PKCS12,
		"CA":          ca.PKCS12,
	} {
		t.Run(name, func(t *testing.T) {
			one, onePassword := encode(t)
			two, twoPassword := encode(t)
			require.NotEqual(t, onePassword, twoPassword)
			require.NotEqual(t, one, two)
			// rand.Text is 26 characters of base32.
			require.Len(t, onePassword, 26)
			require.Regexp(t, `^[A-Z2-7]{26}$`, onePassword)
		})
	}
}

// The parameters of go-pkcs12's Modern2023.
const (
	iterations = 2048
	saltSize   = 16
)

var (
	oidSHA256         = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 1}
	oidHMACWithSHA256 = asn1.ObjectIdentifier{1, 2, 840, 113549, 2, 9}
	oidPBKDF2         = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 5, 12}
	oidPBES2          = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 5, 13}
	oidAES256CBC      = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 1, 42}
	oidData           = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 1}
	oidEncryptedData  = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 6}
	oidShroudedKeyBag = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 12, 10, 1, 2}
	oidLocalKeyID     = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 21}
)

// unmarshal decodes der, which must hold exactly one DER value, into out.
func unmarshal(t *testing.T, der []byte, out any) {
	t.Helper()
	rest, err := asn1.Unmarshal(der, out)
	require.NoError(t, err)
	require.Empty(t, rest)
}

// requirePBES2 asserts that alg is PBES2 with PBKDF2-HMAC-SHA-256 over
// 2048 iterations and a 16-byte salt, and AES-256-CBC with a 16-byte IV.
func requirePBES2(t *testing.T, alg pkix.AlgorithmIdentifier) {
	t.Helper()
	require.Equal(t, oidPBES2, alg.Algorithm)
	var params pkcs12.PBES2Params
	unmarshal(t, alg.Parameters.FullBytes, &params)

	require.Equal(t, oidPBKDF2, params.KeyDerivationFunc.Algorithm)
	var kdf pkcs12.PBKDF2Params
	unmarshal(t, params.KeyDerivationFunc.Parameters.FullBytes, &kdf)
	require.Equal(t, asn1.TagOctetString, kdf.Salt.Tag)
	require.Len(t, kdf.Salt.Bytes, saltSize)
	require.Equal(t, iterations, kdf.IterationCount)
	require.Zero(t, kdf.KeyLength, "no key length, as go-pkcs12 writes")
	require.Equal(t, oidHMACWithSHA256, kdf.PRF.Algorithm)

	require.Equal(t, oidAES256CBC, params.EncryptionScheme.Algorithm)
	var iv []byte
	unmarshal(t, params.EncryptionScheme.Parameters.FullBytes, &iv)
	require.Len(t, iv, 16)
}

// authSafe decodes the PFX in data, requires that its MAC is HMAC-SHA-256
// with RFC 7292's derivation over 2048 iterations and a 16-byte salt, and
// returns its AuthenticatedSafe.
func authSafe(t *testing.T, data []byte) []pkcs12.ContentInfo {
	t.Helper()
	var pfx pkcs12.PFX
	unmarshal(t, data, &pfx)
	require.Equal(t, 3, pfx.Version)
	require.Equal(t, oidSHA256, pfx.MacData.Mac.Algorithm.Algorithm, "the MAC is HMAC-SHA-256")
	require.Len(t, pfx.MacData.Mac.Digest, 32)
	require.Len(t, pfx.MacData.MacSalt, saltSize)
	require.Equal(t, iterations, pfx.MacData.Iterations)

	require.Equal(t, oidData, pfx.AuthSafe.ContentType)
	var octets []byte
	unmarshal(t, pfx.AuthSafe.Content.Bytes, &octets)
	var cis []pkcs12.ContentInfo
	unmarshal(t, octets, &cis)
	return cis
}

// requireEncryptedCertificates requires that ci is an encryptedData
// ContentInfo encrypted with PBES2 as Modern2023 encrypts.
func requireEncryptedCertificates(t *testing.T, ci pkcs12.ContentInfo) {
	t.Helper()
	require.Equal(t, oidEncryptedData, ci.ContentType)
	var ed pkcs12.EncryptedData
	unmarshal(t, ci.Content.Bytes, &ed)
	require.Zero(t, ed.Version)
	require.Equal(t, oidData, ed.EncryptedContentInfo.ContentType)
	requirePBES2(t, ed.EncryptedContentInfo.ContentEncryptionAlgorithm)
	require.False(t, ed.EncryptedContentInfo.EncryptedContent.IsCompound, "the ciphertext is written primitive, as DER requires")
}

func TestPKCS12_encrypts_with_the_algorithms_of_Modern2023(t *testing.T) {
	ca := pkitest.NewCA(t)
	c := ca.Issue(t, "api.internal")

	t.Run("Certificate", func(t *testing.T) {
		bundle, _ := c.PKCS12(t)
		cis := authSafe(t, bundle)
		require.Len(t, cis, 2, "the certificates, then the key")
		requireEncryptedCertificates(t, cis[0])

		require.Equal(t, oidData, cis[1].ContentType, "the key is in a data ContentInfo")
		var octets []byte
		unmarshal(t, cis[1].Content.Bytes, &octets)
		var bags []pkcs12.SafeBag
		unmarshal(t, octets, &bags)
		require.Len(t, bags, 1)
		require.Equal(t, oidShroudedKeyBag, bags[0].ID)
		var info pkcs12.EncryptedPrivateKeyInfo
		unmarshal(t, bags[0].Value.Bytes, &info)
		requirePBES2(t, info.Algorithm)

		require.Len(t, bags[0].Attributes, 1)
		require.Equal(t, oidLocalKeyID, bags[0].Attributes[0].ID)
		var id []byte
		unmarshal(t, bags[0].Attributes[0].Values.Bytes, &id)
		fingerprint := sha1.Sum(c.TLS().Certificate[0])
		require.Equal(t, fingerprint[:], id, "the key's localKeyId is the SHA-1 of the leaf")
	})

	t.Run("CA", func(t *testing.T) {
		bundle, _ := ca.PKCS12(t)
		cis := authSafe(t, bundle)
		require.Len(t, cis, 1, "the certificates alone")
		requireEncryptedCertificates(t, cis[0])
	})
}
