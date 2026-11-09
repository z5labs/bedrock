// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package pkcs12

import (
	"bytes"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"hash"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// This file builds the structures and files the tests decode.

// testPassword is the password of the files the tests build.
const testPassword = "changeit"

// mustMarshal is asn1.Marshal, failing the test on an error.
func mustMarshal(tb testing.TB, v any) []byte {
	tb.Helper()
	der, err := asn1.Marshal(v)
	require.NoError(tb, err)
	return der
}

// wrap returns der in a [0] EXPLICIT tag.
func wrap(der []byte) asn1.RawValue {
	return asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true, Bytes: der}
}

// raw is der as an already encoded value, for an AlgorithmIdentifier's
// parameters.
func raw(der []byte) asn1.RawValue {
	return asn1.RawValue{FullBytes: der}
}

// pw returns the one form of password.
func pw(tb testing.TB, password string) Password {
	tb.Helper()
	pws, err := passwords(password)
	require.NoError(tb, err)
	return pws[0]
}

// testCert is a self-signed certificate and its key.
type testCert struct {
	der []byte
	key *ecdsa.PrivateKey
}

func newTestCert(tb testing.TB) testCert {
	tb.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(tb, err)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "pkcs12 test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(tb, err)
	return testCert{der: der, key: key}
}

// pkcs8 is the key's PKCS#8 DER.
func (c testCert) pkcs8(tb testing.TB) []byte {
	tb.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(c.key)
	require.NoError(tb, err)
	return der
}

// certBagOf returns a certBag holding der.
func certBagOf(tb testing.TB, der []byte, attrs ...Attribute) SafeBag {
	tb.Helper()
	value := mustMarshal(tb, CertBag{ID: OIDX509Certificate, Value: wrap(mustMarshal(tb, der))})
	return SafeBag{ID: OIDCertBag, Value: wrap(value), Attributes: attrs}
}

// dataInfo returns a data ContentInfo that holds der as its OCTET STRING.
func dataInfo(tb testing.TB, der []byte) ContentInfo {
	tb.Helper()
	return ContentInfo{ContentType: OIDData, Content: wrap(mustMarshal(tb, der))}
}

// safeContentsOf returns the DER of a SafeContents of bags.
func safeContentsOf(tb testing.TB, bags ...SafeBag) []byte {
	tb.Helper()
	if bags == nil {
		bags = []SafeBag{}
	}
	return mustMarshal(tb, bags)
}

// encryptedInfo returns an encryptedData ContentInfo of ed.
func encryptedInfo(tb testing.TB, ed EncryptedData) ContentInfo {
	tb.Helper()
	return ContentInfo{ContentType: OIDEncryptedData, Content: wrap(mustMarshal(tb, ed))}
}

// authSafeOf returns the DER of an AuthenticatedSafe of cis.
func authSafeOf(tb testing.TB, cis ...ContentInfo) []byte {
	tb.Helper()
	if cis == nil {
		cis = []ContentInfo{}
	}
	return mustMarshal(tb, cis)
}

// pfxOf returns a PFX of authSafe, with mac.
func pfxOf(tb testing.TB, authSafe []byte, mac MacData) []byte {
	tb.Helper()
	return mustMarshal(tb, PFX{Version: 3, AuthSafe: dataInfo(tb, authSafe), MacData: mac})
}

// macOf returns a MacData of authSafe, an HMAC with h keyed by RFC 7292's
// derivation from password, the BMPString itself, over 2048 iterations.
func macOf(authSafe []byte, oid asn1.ObjectIdentifier, h func() hash.Hash, password []byte) MacData {
	salt := []byte("saltsalt")
	mac := hmac.New(h, Derive(h, password, salt, 2048, purposeMAC, h().Size()))
	mac.Write(authSafe)
	return MacData{
		Mac:        DigestInfo{Algorithm: algorithm{Algorithm: oid, Parameters: asn1.NullRawValue}, Digest: mac.Sum(nil)},
		MacSalt:    salt,
		Iterations: 2048,
	}
}

// sha1MacOf is macOf with HMAC-SHA-1 and password's one form.
func sha1MacOf(tb testing.TB, authSafe []byte, password string) MacData {
	tb.Helper()
	return macOf(authSafe, OIDSHA1, sha1.New, pw(tb, password).bmp)
}

// pbkdf2Alg returns PBKDF2 with p as its parameters.
func pbkdf2Alg(tb testing.TB, p PBKDF2Params) algorithm {
	tb.Helper()
	return algorithm{Algorithm: OIDPBKDF2, Parameters: raw(mustMarshal(tb, p))}
}

// pbes2Alg returns PBES2 with kdf and AES scheme, whose IV is iv.
func pbes2Alg(tb testing.TB, kdf algorithm, scheme asn1.ObjectIdentifier, iv []byte) algorithm {
	tb.Helper()
	return algorithm{Algorithm: OIDPBES2, Parameters: raw(mustMarshal(tb, PBES2Params{
		KeyDerivationFunc: kdf,
		EncryptionScheme:  algorithm{Algorithm: scheme, Parameters: raw(mustMarshal(tb, iv))},
	}))}
}

// pad adds PKCS#7 padding to b for blocks of size.
func pad(b []byte, size int) []byte {
	n := size - len(b)%size
	return append(append([]byte{}, b...), bytes.Repeat([]byte{byte(n)}, n)...)
}

// encryptCBC encrypts plaintext, padded, in CBC mode with block and iv.
func encryptCBC(block cipher.Block, iv, plaintext []byte) []byte {
	out := pad(plaintext, block.BlockSize())
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(out, out)
	return out
}

// pbes2Encrypt encrypts plaintext with PBES2 as alg names it, with an
// AES key of size bytes, derived from password with prf.
func pbes2Encrypt(tb testing.TB, prf func() hash.Hash, salt []byte, size int, iv []byte, password string, plaintext []byte) []byte {
	tb.Helper()
	k := pbkdf2Key{salt: salt, iterations: 2048, prf: prf}
	key, err := k.derive([]byte(password), size)
	require.NoError(tb, err)
	block, err := aes.NewCipher(key)
	require.NoError(tb, err)
	return encryptCBC(block, iv, plaintext)
}

// shroud returns a pkcs8ShroudedKeyBag of der, encrypted with PBES2, with
// PBKDF2-HMAC-SHA-1 named by default and AES-128-CBC.
func shroud(tb testing.TB, der []byte, password string) SafeBag {
	tb.Helper()
	salt, iv := []byte("saltsalt"), bytes.Repeat([]byte{7}, aes.BlockSize)
	alg := pbes2Alg(tb, pbkdf2Alg(tb, PBKDF2Params{Salt: asn1.RawValue{Tag: asn1.TagOctetString, Bytes: salt}, IterationCount: 2048}), OIDAES128CBC, iv)
	info := EncryptedPrivateKeyInfo{Algorithm: alg, EncryptedData: pbes2Encrypt(tb, sha1.New, salt, 16, iv, password, der)}
	return SafeBag{ID: OIDPKCS8ShroudedKeyBag, Value: wrap(mustMarshal(tb, info))}
}

// equalKey reports whether key is the key of c.
func (c testCert) equalKey(key crypto.PrivateKey) bool {
	return c.key.Equal(key)
}
