// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package pkcs12

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/des"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/asn1"
	"hash"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	sslmate "software.sslmate.com/src/go-pkcs12"
)

// requirePrefix checks that err's message starts with prefix.
func requirePrefix(t *testing.T, err error, prefix string) {
	t.Helper()
	require.Error(t, err)
	require.True(t, strings.HasPrefix(err.Error(), prefix), "the error %q starts with %q", err, prefix)
}

func TestName(t *testing.T) {
	require.Equal(t, "pbeWithSHAAnd40BitRC2-CBC (1.2.840.113549.1.12.1.6)", Name(OIDPBEWithSHAAnd40BitRC2CBC))
	require.Equal(t, "1.2.3", Name(asn1.ObjectIdentifier{1, 2, 3}))
}

func TestUnmarshal_fails_on_trailing_bytes(t *testing.T) {
	var n int
	require.EqualError(t, unmarshal([]byte{2, 1, 5, 0}, &n), "1 trailing bytes")
	requirePrefix(t, unmarshal(nil, &n), "asn1: ")
	require.NoError(t, unmarshal([]byte{2, 1, 5}, &n))
	require.Equal(t, 5, n)
}

func TestPasswords(t *testing.T) {
	pws, err := passwords("")
	require.NoError(t, err)
	require.Equal(t, []Password{{bmp: []byte{0, 0}}, {}}, pws, "the terminator alone, and then zero bytes")

	pws, err = passwords("pä密")
	require.NoError(t, err)
	require.Equal(t, []Password{{bmp: []byte{0, 'p', 0, 0xe4, 0x5b, 0xc6, 0, 0}, utf8: []byte("pä密")}}, pws)

	_, err = passwords("\xff")
	require.EqualError(t, err, "the password isn't valid UTF-8")
	_, err = passwords("p😀")
	require.EqualError(t, err, "the password holds a character outside the Basic Multilingual Plane, which a BMPString can't hold")
}

func TestDerive(t *testing.T) {
	t.Run("panics on fewer than 1 iteration", func(t *testing.T) {
		require.PanicsWithValue(t, "pkcs12: Derive called with 0 iterations", func() {
			Derive(sha1.New, nil, nil, 0, purposeKey, 24)
		})
	})

	t.Run("one block and many", func(t *testing.T) {
		long := Derive(sha1.New, []byte{0, 'a', 0, 0}, []byte("salt"), 3, purposeKey, 100)
		short := Derive(sha1.New, []byte{0, 'a', 0, 0}, []byte("salt"), 3, purposeKey, 20)
		require.Len(t, long, 100)
		require.Equal(t, short, long[:20], "the first block doesn't depend on the size")
		require.NotEqual(t, short, Derive(sha1.New, []byte{0, 'a', 0, 0}, []byte("salt"), 3, purposeIV, 20), "the purpose diversifies")
	})

	t.Run("the empty password and salt", func(t *testing.T) {
		require.Len(t, Derive(sha256.New, nil, nil, 1, purposeMAC, 32), 32)
	})
}

func TestAddOne_carries(t *testing.T) {
	x := []byte{0x00, 0xff, 0xff}
	addOne(x, []byte{0x00, 0x00, 0x00})
	require.Equal(t, []byte{0x01, 0x00, 0x00}, x)
	x = []byte{0xff, 0xff}
	addOne(x, []byte{0x00, 0x00})
	require.Equal(t, []byte{0x00, 0x00}, x, "modulo 2 to the power of its length")
}

func TestCheckIterations(t *testing.T) {
	require.NoError(t, checkIterations(1))
	require.NoError(t, checkIterations(MaxIterations))
	require.EqualError(t, checkIterations(0), "iteration count 0, outside 1 to 5000000")
	require.EqualError(t, checkIterations(MaxIterations+1), "iteration count 5000001, outside 1 to 5000000")
}

// octetSalt is salt as an OCTET STRING, for PBKDF2's parameters.
func octetSalt(salt []byte) asn1.RawValue {
	return asn1.RawValue{Tag: asn1.TagOctetString, Bytes: salt}
}

func TestPBKDF2Params(t *testing.T) {
	salt := octetSalt([]byte("saltsalt"))
	for _, row := range []struct {
		name    string
		alg     algorithm
		wantErr string
	}{
		{"not PBKDF2", algorithm{Algorithm: OIDPBES2}, "unsupported key derivation function PBES2 (1.2.840.113549.1.5.13)"},
		{"no parameters", algorithm{Algorithm: OIDPBKDF2}, "PBKDF2 parameters: asn1: "},
		{"a salt from another source", pbkdf2Alg(t, PBKDF2Params{Salt: raw(mustMarshal(t, algorithm{Algorithm: OIDSHA1})), IterationCount: 1}),
			"PBKDF2: a salt that isn't an OCTET STRING"},
		{"no iterations", pbkdf2Alg(t, PBKDF2Params{Salt: salt, IterationCount: 0}), "PBKDF2: iteration count 0, outside 1 to 5000000"},
		{"too many iterations", pbkdf2Alg(t, PBKDF2Params{Salt: salt, IterationCount: MaxIterations + 1}), "PBKDF2: iteration count 5000001, outside 1 to 5000000"},
		{"an unknown PRF", pbkdf2Alg(t, PBKDF2Params{Salt: salt, IterationCount: 1, PRF: algorithm{Algorithm: OIDSHA256}}),
			"unsupported PBKDF2 PRF SHA-256 (2.16.840.1.101.3.4.2.1)"},
	} {
		t.Run(row.name, func(t *testing.T) {
			_, err := pbkdf2Params(row.alg)
			requirePrefix(t, err, row.wantErr)
		})
	}

	t.Run("the PRF is hmacWithSHA1 by default", func(t *testing.T) {
		k, err := pbkdf2Params(pbkdf2Alg(t, PBKDF2Params{Salt: salt, IterationCount: 7, KeyLength: 16}))
		require.NoError(t, err)
		require.Equal(t, []byte("saltsalt"), k.salt)
		require.Equal(t, 7, k.iterations)
		require.Equal(t, 16, k.keyLength)
		require.Equal(t, sha1.New().Size(), k.prf().Size())
	})

	t.Run("a named PRF", func(t *testing.T) {
		k, err := pbkdf2Params(pbkdf2Alg(t, PBKDF2Params{Salt: salt, IterationCount: 7, PRF: algorithm{Algorithm: OIDHMACWithSHA384}}))
		require.NoError(t, err)
		require.Equal(t, sha512.New384().Size(), k.prf().Size())
	})
}

func TestDecryptCBC(t *testing.T) {
	key, iv := bytes.Repeat([]byte{1}, 16), bytes.Repeat([]byte{2}, 16)
	block, err := aes.NewCipher(key)
	require.NoError(t, err)
	encrypt := func(b []byte) []byte { return encryptRaw(t, key, iv, b) }

	got, err := decryptCBC(aes.NewCipher, key, iv, encryptCBC(block, iv, []byte("plaintext")))
	require.NoError(t, err)
	require.Equal(t, []byte("plaintext"), got)

	for _, row := range []struct {
		name    string
		key, iv []byte
		data    []byte
		wantErr string
	}{
		{"a key of the wrong size", key[:3], iv, nil, "crypto/aes: invalid key size 3"},
		{"an IV of the wrong size", key, iv[:8], nil, "an IV of 8 bytes, not 16"},
		{"no ciphertext", key, iv, nil, "0 bytes of ciphertext, not a positive multiple of 16"},
		{"ciphertext of the wrong length", key, iv, make([]byte, 17), "17 bytes of ciphertext, not a positive multiple of 16"},
		{"a padding of 0", key, iv, encrypt(append(make([]byte, 15), 0)), "padding that isn't valid PKCS#7"},
		{"a padding longer than a block", key, iv, encrypt(append(make([]byte, 15), 17)), "padding that isn't valid PKCS#7"},
		{"a padding of unequal bytes", key, iv, encrypt(append(make([]byte, 14), 1, 2)), "padding that isn't valid PKCS#7"},
	} {
		t.Run(row.name, func(t *testing.T) {
			got, err := decryptCBC(aes.NewCipher, row.key, row.iv, row.data)
			require.EqualError(t, err, row.wantErr)
			require.Nil(t, got)
		})
	}
}

// encryptRaw encrypts b, a whole number of blocks, with AES-CBC and no
// padding.
func encryptRaw(t *testing.T, key, iv, b []byte) []byte {
	t.Helper()
	block, err := aes.NewCipher(key)
	require.NoError(t, err)
	out := make([]byte, len(b))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(out, b)
	return out
}

func TestDecrypt(t *testing.T) {
	plaintext := []byte("a SafeContents")
	salt, iv := []byte("saltsalt"), bytes.Repeat([]byte{3}, 16)
	p := pw(t, "pä密")
	kdf := func(prf algorithm, keyLength int) algorithm {
		return pbkdf2Alg(t, PBKDF2Params{Salt: octetSalt(salt), IterationCount: 2048, KeyLength: keyLength, PRF: prf})
	}

	t.Run("PBES2", func(t *testing.T) {
		for _, row := range []struct {
			name   string
			prf    algorithm
			hash   func() hash.Hash
			scheme asn1.ObjectIdentifier
			size   int
		}{
			{"hmacWithSHA1 by default, AES-128-CBC", algorithm{}, sha1.New, OIDAES128CBC, 16},
			{"hmacWithSHA256, AES-192-CBC", algorithm{Algorithm: OIDHMACWithSHA256}, sha256.New, OIDAES192CBC, 24},
			{"hmacWithSHA384, AES-256-CBC", algorithm{Algorithm: OIDHMACWithSHA384}, sha512.New384, OIDAES256CBC, 32},
			{"hmacWithSHA512, AES-256-CBC", algorithm{Algorithm: OIDHMACWithSHA512}, sha512.New, OIDAES256CBC, 32},
		} {
			t.Run(row.name, func(t *testing.T) {
				alg := pbes2Alg(t, kdf(row.prf, row.size), row.scheme, iv)
				data := pbes2Encrypt(t, row.hash, salt, row.size, iv, "pä密", plaintext)
				got, err := Decrypt(alg, p, data)
				require.NoError(t, err)
				require.Equal(t, plaintext, got)
			})
		}
	})

	t.Run("pbeWithSHAAnd3-KeyTripleDES-CBC", func(t *testing.T) {
		params := PBEParams{Salt: salt, Iterations: 2048}
		block, err := des.NewTripleDESCipher(Derive(sha1.New, p.bmp, salt, 2048, purposeKey, 24))
		require.NoError(t, err)
		data := encryptCBC(block, Derive(sha1.New, p.bmp, salt, 2048, purposeIV, 8), plaintext)
		got, err := Decrypt(algorithm{Algorithm: OIDPBEWithSHAAnd3KeyTripleDESCBC, Parameters: raw(mustMarshal(t, params))}, p, data)
		require.NoError(t, err)
		require.Equal(t, plaintext, got)
	})

	for _, row := range []struct {
		name    string
		alg     algorithm
		wantErr string
	}{
		{"RC2 inside PBES2", pbes2Alg(t, kdf(algorithm{}, 0), asn1.ObjectIdentifier{1, 2, 840, 113549, 3, 2}, iv[:8]),
			"unsupported PBES2 encryption scheme 1.2.840.113549.3.2"},
		{"RC2 with no parameters", algorithm{Algorithm: OIDPBEWithSHAAnd40BitRC2CBC}, "pbeWithSHAAnd40BitRC2-CBC parameters: asn1: "},
		{"RC2 with too many iterations", algorithm{Algorithm: OIDPBEWithSHAAnd128BitRC2CBC, Parameters: raw(mustMarshal(t, PBEParams{Salt: salt, Iterations: MaxIterations + 1}))},
			"pbeWithSHAAnd128BitRC2-CBC: iteration count 5000001, outside 1 to 5000000"},
		{"RC4", algorithm{Algorithm: OIDPBEWithSHAAnd128BitRC4}, "unsupported encryption algorithm pbeWithSHAAnd128BitRC4 (1.2.840.113549.1.12.1.1)"},
		{"2-key 3DES", algorithm{Algorithm: OIDPBEWithSHAAnd2KeyTripleDESCBC}, "unsupported encryption algorithm pbeWithSHAAnd2-KeyTripleDES-CBC (1.2.840.113549.1.12.1.4)"},
		{"PBES1", algorithm{Algorithm: asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 5, 10}}, "unsupported encryption algorithm pbeWithSHA1AndDES-CBC (PBES1) (1.2.840.113549.1.5.10)"},
		{"3DES with no parameters", algorithm{Algorithm: OIDPBEWithSHAAnd3KeyTripleDESCBC}, "pbeWithSHAAnd3-KeyTripleDES-CBC parameters: asn1: "},
		{"3DES with too many iterations", algorithm{Algorithm: OIDPBEWithSHAAnd3KeyTripleDESCBC, Parameters: raw(mustMarshal(t, PBEParams{Salt: salt, Iterations: MaxIterations + 1}))},
			"pbeWithSHAAnd3-KeyTripleDES-CBC: iteration count 5000001, outside 1 to 5000000"},
		{"PBES2 with no parameters", algorithm{Algorithm: OIDPBES2}, "PBES2 parameters: asn1: "},
		{"PBES2 with scrypt", pbes2Alg(t, algorithm{Algorithm: asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 11591, 4, 11}}, OIDAES256CBC, iv),
			"PBES2: unsupported key derivation function 1.3.6.1.4.1.11591.4.11"},
		{"PBES2 with DES-EDE3-CBC", pbes2Alg(t, kdf(algorithm{}, 0), asn1.ObjectIdentifier{1, 2, 840, 113549, 3, 7}, iv),
			"unsupported PBES2 encryption scheme 1.2.840.113549.3.7"},
		{"PBES2 with a key length that isn't the cipher's", pbes2Alg(t, kdf(algorithm{}, 16), OIDAES256CBC, iv),
			"PBES2: PBKDF2 key length 16, not the 32 bytes of AES-256-CBC (2.16.840.1.101.3.4.1.42)"},
		{"PBES2 with an IV that isn't an OCTET STRING", algorithm{Algorithm: OIDPBES2, Parameters: raw(mustMarshal(t, PBES2Params{
			KeyDerivationFunc: kdf(algorithm{}, 0),
			EncryptionScheme:  algorithm{Algorithm: OIDAES128CBC, Parameters: asn1.NullRawValue},
		}))}, "PBES2: AES-128-CBC (2.16.840.1.101.3.4.1.2) IV: asn1: "},
		{"PBES2 with a short IV", pbes2Alg(t, kdf(algorithm{}, 0), OIDAES128CBC, iv[:8]), "an IV of 8 bytes, not 16"},
	} {
		t.Run(row.name, func(t *testing.T) {
			got, err := Decrypt(row.alg, p, make([]byte, 16))
			requirePrefix(t, err, row.wantErr)
			require.Nil(t, got)
		})
	}
}

// fixture reads a file from testdata.
func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	return data
}

func TestDecode_verifies_every_MAC_algorithm(t *testing.T) {
	cert := newTestCert(t)
	safe := authSafeOf(t, dataInfo(t, safeContentsOf(t, certBagOf(t, cert.der))))
	bmp := pw(t, testPassword).bmp

	rows := []struct {
		name string
		data []byte
	}{
		// The OpenSSL fixtures check RFC 7292's derivation with a 128-byte
		// block, which go-pkcs12 can't write. Each was written by OpenSSL
		// 3.5.9 for an EC key and a self-signed certificate, with password
		// "1234": openssl pkcs12 -export -passout pass:1234 -macalg sha384.
		{"HMAC-SHA-384, from OpenSSL", fixture(t, "openssl-mac-sha384.p12")},
		// openssl pkcs12 -export -passout pass:1234 -macalg sha512.
		{"HMAC-SHA-512, from OpenSSL", fixture(t, "openssl-mac-sha512.p12")},
		// openssl pkcs12 -export -passout pass:1234 -macalg sha384
		// -pbmac1_pbkdf2 -pbmac1_pbkdf2_md sha384.
		{"PBMAC1 with HMAC-SHA-384, from OpenSSL", fixture(t, "openssl-pbmac1-sha384.p12")},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			bags, _, err := Decode(row.data, "1234")
			require.NoError(t, err)
			require.Len(t, bags, 2)
			_, _, err = Decode(row.data, "4321")
			require.ErrorIs(t, err, ErrMAC)
		})
	}

	for _, row := range []struct {
		name string
		mac  MacData
	}{
		{"HMAC-SHA-1", macOf(safe, OIDSHA1, sha1.New, bmp)},
		{"HMAC-SHA-256", macOf(safe, OIDSHA256, sha256.New, bmp)},
		{"HMAC-SHA-384", macOf(safe, OIDSHA384, sha512.New384, bmp)},
		{"HMAC-SHA-512", macOf(safe, OIDSHA512, sha512.New, bmp)},
	} {
		t.Run(row.name, func(t *testing.T) {
			bags, got, err := Decode(pfxOf(t, safe, row.mac), testPassword)
			require.NoError(t, err)
			require.Len(t, bags, 1)
			require.Equal(t, pw(t, testPassword), got)
		})
	}
}

func TestDecode_tries_both_forms_of_the_empty_password(t *testing.T) {
	cert := newTestCert(t)
	safe := authSafeOf(t, dataInfo(t, safeContentsOf(t, certBagOf(t, cert.der))))
	for _, row := range []struct {
		name string
		form Password
	}{
		{"the terminator alone", Password{bmp: []byte{0, 0}}},
		{"zero bytes, as Windows writes it", Password{}},
	} {
		t.Run(row.name, func(t *testing.T) {
			_, got, err := Decode(pfxOf(t, safe, macOf(safe, OIDSHA256, sha256.New, row.form.bmp)), "")
			require.NoError(t, err)
			require.Equal(t, row.form, got, "the form whose MAC verifies is the one used to decrypt")
		})
	}

	t.Run("a file with no MAC", func(t *testing.T) {
		_, got, err := Decode(pfxOf(t, safe, MacData{}), "")
		require.NoError(t, err)
		require.Equal(t, Password{bmp: []byte{0, 0}}, got)
	})
}

func TestDecode_decodes_go_pkcs12(t *testing.T) {
	cert := newTestCert(t)
	parsed, err := x509.ParseCertificate(cert.der)
	require.NoError(t, err)
	for _, row := range []struct {
		name string
		enc  *sslmate.Encoder
	}{
		{"LegacyRC2", sslmate.LegacyRC2},
		{"LegacyDES", sslmate.LegacyDES},
		{"Modern2023", sslmate.Modern2023},
		{"Modern2026", sslmate.Modern2026},
		{"Passwordless", sslmate.Passwordless},
	} {
		t.Run(row.name, func(t *testing.T) {
			password := testPassword
			if row.enc == sslmate.Passwordless {
				password = ""
			}
			data, err := row.enc.Encode(cert.key, parsed, nil, password)
			require.NoError(t, err)

			bags, p, err := Decode(data, password)
			require.NoError(t, err)
			require.Len(t, bags, 2)
			require.True(t, bags[0].ID.Equal(OIDCertBag))
			der, err := bags[0].Certificate()
			require.NoError(t, err)
			require.Equal(t, cert.der, der)
			require.False(t, bags[0].Trusted())

			key, err := bags[1].PrivateKey(p)
			require.NoError(t, err)
			parsedKey, err := x509.ParsePKCS8PrivateKey(key)
			require.NoError(t, err)
			require.True(t, cert.equalKey(parsedKey))
		})
	}

	t.Run("a trust store", func(t *testing.T) {
		data, err := sslmate.Passwordless.EncodeTrustStore([]*x509.Certificate{parsed}, "")
		require.NoError(t, err)
		bags, _, err := Decode(data, "")
		require.NoError(t, err)
		require.Len(t, bags, 1)
		require.True(t, bags[0].Trusted())
	})
}

func TestDecode_fails(t *testing.T) {
	cert := newTestCert(t)
	good := safeContentsOf(t, certBagOf(t, cert.der))
	withMAC := func(safe []byte) []byte { return pfxOf(t, safe, sha1MacOf(t, safe, testPassword)) }
	encrypted := func(ed EncryptedData) []byte { return withMAC(authSafeOf(t, encryptedInfo(t, ed))) }
	unsupportedAlg := algorithm{Algorithm: OIDPBEWithSHAAnd128BitRC4}
	pfxWith := func(p PFX) []byte { return mustMarshal(t, p) }
	pbmac1With := func(params []byte) []byte {
		safe := authSafeOf(t, dataInfo(t, good))
		return pfxOf(t, safe, MacData{Mac: DigestInfo{Algorithm: algorithm{Algorithm: OIDPBMAC1, Parameters: raw(params)}, Digest: make([]byte, 32)}, MacSalt: []byte{}, Iterations: 1})
	}
	pbmac1Params := func(keyLength int, mac asn1.ObjectIdentifier) []byte {
		return mustMarshal(t, PBMAC1Params{
			KeyDerivationFunc: pbkdf2Alg(t, PBKDF2Params{Salt: octetSalt([]byte("saltsalt")), IterationCount: 1, KeyLength: keyLength}),
			MessageAuthScheme: algorithm{Algorithm: mac},
		})
	}

	for _, row := range []struct {
		name     string
		data     []byte
		password string
		wantErr  string
	}{
		{"not BER", []byte("PKCS#12"), testPassword, "PFX: BER at offset 1: a length past the end of the input"},
		{"not a PFX", mustMarshal(t, 1), testPassword, "PFX: asn1: "},
		{"an AuthenticatedSafe that isn't BER", withMAC([]byte{0x30, 0x80}), testPassword, "AuthenticatedSafe: BER at offset 0: a missing end-of-contents"},
		{"a SafeContents that isn't BER", withMAC(authSafeOf(t, dataInfo(t, []byte{0x30, 0x80}))), testPassword,
			"AuthenticatedSafe: ContentInfo 1: SafeContents: BER at offset 0: a missing end-of-contents"},
		{"trailing bytes", append(withMAC(authSafeOf(t, dataInfo(t, good))), 0), testPassword, "PFX: 1 trailing bytes"},
		{"version 2", pfxWith(PFX{Version: 2, AuthSafe: dataInfo(t, authSafeOf(t))}), "", "PFX: version 2, not 3"},
		{"a signed authSafe", pfxWith(PFX{Version: 3, AuthSafe: ContentInfo{ContentType: asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 2}, Content: wrap(mustMarshal(t, 1))}}), "",
			"authSafe: content type signedData (1.2.840.113549.1.7.2), not data"},
		{"an authSafe that isn't an OCTET STRING", pfxWith(PFX{Version: 3, AuthSafe: ContentInfo{ContentType: OIDData, Content: wrap(mustMarshal(t, 1))}}), "",
			"authSafe: asn1: "},
		{"a password with a character outside the BMP", withMAC(authSafeOf(t)), "😀", "the password holds a character outside the Basic Multilingual Plane"},
		{"a password for a file with no MAC", pfxOf(t, authSafeOf(t), MacData{}), testPassword,
			"a password for a file with no macData, which decodes only with the empty password"},
		{"an unknown MAC", pfxOf(t, authSafeOf(t), MacData{Mac: DigestInfo{Algorithm: algorithm{Algorithm: asn1.ObjectIdentifier{1, 2, 840, 113549, 2, 5}}}, Iterations: 1}), testPassword,
			"macData: unsupported MAC algorithm 1.2.840.113549.2.5"},
		{"a MAC with no iterations", pfxOf(t, authSafeOf(t), MacData{Mac: DigestInfo{Algorithm: algorithm{Algorithm: OIDSHA256}}, Iterations: -1}), testPassword,
			"macData: iteration count -1, outside 1 to 5000000"},
		{"a MAC that doesn't verify", pfxOf(t, authSafeOf(t), MacData{Mac: DigestInfo{Algorithm: algorithm{Algorithm: OIDSHA256}, Digest: make([]byte, 32)}, Iterations: 1}), testPassword,
			"macData: the MAC doesn't verify with the password"},
		{"PBMAC1 with no parameters", pbmac1With(nil), testPassword, "macData: PBMAC1 parameters: asn1: "},
		{"PBMAC1 with scrypt", pbmac1With(mustMarshal(t, PBMAC1Params{KeyDerivationFunc: algorithm{Algorithm: asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 11591, 4, 11}}, MessageAuthScheme: algorithm{Algorithm: OIDHMACWithSHA256}})), testPassword,
			"macData: PBMAC1: unsupported key derivation function 1.3.6.1.4.1.11591.4.11"},
		{"PBMAC1 with no key length", pbmac1With(pbmac1Params(0, OIDHMACWithSHA256)), testPassword, "macData: PBMAC1: PBKDF2 has no key length"},
		{"PBMAC1 with a key length of 19", pbmac1With(pbmac1Params(19, OIDHMACWithSHA256)), testPassword, "macData: PBMAC1: PBKDF2 key length 19, outside 20 to 64"},
		{"PBMAC1 with an unknown MAC", pbmac1With(pbmac1Params(32, OIDSHA256)), testPassword,
			"macData: unsupported PBMAC1 MAC SHA-256 (2.16.840.1.101.3.4.2.1)"},
		{"PBMAC1 that doesn't verify", pbmac1With(pbmac1Params(64, OIDHMACWithSHA512)), testPassword, "macData: the MAC doesn't verify with the password"},
		{"an AuthenticatedSafe that isn't a SEQUENCE", withMAC(mustMarshal(t, 1)), testPassword, "AuthenticatedSafe: asn1: "},
		{"a ContentInfo of another type", withMAC(authSafeOf(t, dataInfo(t, good), ContentInfo{ContentType: asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 3}, Content: wrap(mustMarshal(t, 1))})), testPassword,
			"AuthenticatedSafe: ContentInfo 2: content type envelopedData (1.2.840.113549.1.7.3), neither data nor encryptedData"},
		{"data that isn't an OCTET STRING", withMAC(authSafeOf(t, ContentInfo{ContentType: OIDData, Content: wrap(mustMarshal(t, 1))})), testPassword,
			"AuthenticatedSafe: ContentInfo 1: data: asn1: "},
		{"encryptedData that isn't EncryptedData", withMAC(authSafeOf(t, ContentInfo{ContentType: OIDEncryptedData, Content: wrap(mustMarshal(t, 1))})), testPassword,
			"AuthenticatedSafe: ContentInfo 1: encryptedData: asn1: "},
		{"encryptedData of version 1", encrypted(EncryptedData{Version: 1, EncryptedContentInfo: EncryptedContentInfo{ContentType: OIDData, ContentEncryptionAlgorithm: unsupportedAlg, EncryptedContent: implicit([]byte{1})}}), testPassword,
			"AuthenticatedSafe: ContentInfo 1: encryptedData: version 1, not 0"},
		{"encryptedData of another content type", encrypted(EncryptedData{EncryptedContentInfo: EncryptedContentInfo{ContentType: OIDEncryptedData, ContentEncryptionAlgorithm: unsupportedAlg, EncryptedContent: implicit([]byte{1})}}), testPassword,
			"AuthenticatedSafe: ContentInfo 1: encryptedData: content type encryptedData (1.2.840.113549.1.7.6), not data"},
		{"encryptedData with RC4", encrypted(EncryptedData{EncryptedContentInfo: EncryptedContentInfo{ContentType: OIDData, ContentEncryptionAlgorithm: unsupportedAlg, EncryptedContent: implicit([]byte{1})}}), testPassword,
			"AuthenticatedSafe: ContentInfo 1: encryptedData: unsupported encryption algorithm pbeWithSHAAnd128BitRC4"},
		{"encryptedContent of segments that aren't OCTET STRINGs", encrypted(EncryptedData{EncryptedContentInfo: EncryptedContentInfo{ContentType: OIDData, ContentEncryptionAlgorithm: unsupportedAlg,
			EncryptedContent: asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true, Bytes: mustMarshal(t, 1)}}}), testPassword,
			"AuthenticatedSafe: ContentInfo 1: encryptedData: encryptedContent: asn1: "},
		{"a SafeContents that isn't a SEQUENCE", withMAC(authSafeOf(t, dataInfo(t, mustMarshal(t, 1)))), testPassword,
			"AuthenticatedSafe: ContentInfo 1: SafeContents: asn1: "},
	} {
		t.Run(row.name, func(t *testing.T) {
			bags, got, err := Decode(row.data, row.password)
			requirePrefix(t, err, row.wantErr)
			require.Nil(t, bags)
			require.Equal(t, Password{}, got)
		})
	}
}

func TestDecode_decrypts_encryptedData(t *testing.T) {
	cert := newTestCert(t)
	salt, iv := []byte("saltsalt"), bytes.Repeat([]byte{5}, 16)
	alg := pbes2Alg(t, pbkdf2Alg(t, PBKDF2Params{Salt: octetSalt(salt), IterationCount: 2048, PRF: algorithm{Algorithm: OIDHMACWithSHA256}}), OIDAES256CBC, iv)
	plaintext := safeContentsOf(t, certBagOf(t, cert.der), shroud(t, cert.pkcs8(t), testPassword))
	ed := EncryptedData{EncryptedContentInfo: EncryptedContentInfo{
		ContentType:                OIDData,
		ContentEncryptionAlgorithm: alg,
		EncryptedContent:           implicit(pbes2Encrypt(t, sha256.New, salt, 32, iv, testPassword, plaintext)),
	}}
	safe := authSafeOf(t, encryptedInfo(t, ed), dataInfo(t, safeContentsOf(t)), dataInfo(t, safeContentsOf(t, certBagOf(t, cert.der))))

	bags, p, err := Decode(pfxOf(t, safe, sha1MacOf(t, safe, testPassword)), testPassword)
	require.NoError(t, err)
	require.Len(t, bags, 3, "an AuthenticatedSafe may hold any number of ContentInfos")
	key, err := bags[1].PrivateKey(p)
	require.NoError(t, err)
	parsed, err := x509.ParsePKCS8PrivateKey(key)
	require.NoError(t, err)
	require.True(t, cert.equalKey(parsed))
}

func TestDecode_joins_a_constructed_encryptedContent(t *testing.T) {
	cert := newTestCert(t)
	salt, iv := []byte("saltsalt"), bytes.Repeat([]byte{5}, 16)
	alg := pbes2Alg(t, pbkdf2Alg(t, PBKDF2Params{Salt: octetSalt(salt), IterationCount: 2048}), OIDAES128CBC, iv)
	plaintext := indefinite(safeContentsOf(t, certBagOf(t, cert.der)))
	ciphertext := pbes2Encrypt(t, sha1.New, salt, 16, iv, testPassword, plaintext)
	// The segments of an OCTET STRING with an implicit [0] tag, as BER
	// writes them.
	var segments []byte
	for i := 0; i < len(ciphertext); i += 16 {
		segments = append(segments, mustMarshal(t, ciphertext[i:i+16])...)
	}
	ed := EncryptedData{EncryptedContentInfo: EncryptedContentInfo{
		ContentType:                OIDData,
		ContentEncryptionAlgorithm: alg,
		EncryptedContent:           asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true, Bytes: segments},
	}}
	safe := authSafeOf(t, encryptedInfo(t, ed))

	bags, _, err := Decode(pfxOf(t, safe, sha1MacOf(t, safe, testPassword)), testPassword)
	require.NoError(t, err)
	require.Len(t, bags, 1)
	der, err := bags[0].Certificate()
	require.NoError(t, err)
	require.Equal(t, cert.der, der, "the decrypted SafeContents is rewritten to DER")
}

func TestDecode_decrypts_RC2_from_OpenSSL(t *testing.T) {
	// Written by OpenSSL 3.5.9 for an EC key and a self-signed certificate:
	// openssl pkcs12 -export -legacy -passout pass:1234
	// -certpbe PBE-SHA1-RC2-128 -keypbe PBE-SHA1-RC2-128.
	bags, p, err := Decode(fixture(t, "openssl-rc2-128.p12"), "1234")
	require.NoError(t, err)
	require.Len(t, bags, 2)
	der, err := bags[0].Certificate()
	require.NoError(t, err)
	_, err = x509.ParseCertificate(der)
	require.NoError(t, err)
	key, err := bags[1].PrivateKey(p)
	require.NoError(t, err)
	_, err = x509.ParsePKCS8PrivateKey(key)
	require.NoError(t, err)
}

// indefinite returns der, a DER value of a one-octet identifier, with an
// indefinite length.
func indefinite(der []byte) []byte {
	return append(append([]byte{der[0], 0x80}, der[headerLength(der):]...), 0, 0)
}

func TestSafeBag_Certificate_fails(t *testing.T) {
	cert := newTestCert(t)
	for _, row := range []struct {
		name    string
		bag     SafeBag
		wantErr string
	}{
		{"not a CertBag", SafeBag{ID: OIDCertBag, Value: wrap(mustMarshal(t, 1))}, "asn1: "},
		{"an SDSI certificate", SafeBag{ID: OIDCertBag, Value: wrap(mustMarshal(t, CertBag{ID: asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 22, 2}, Value: wrap(mustMarshal(t, cert.der))}))},
			"a certificate of type sdsiCertificate (1.2.840.113549.1.9.22.2), not x509Certificate"},
		{"a certificate that isn't an OCTET STRING", SafeBag{ID: OIDCertBag, Value: wrap(mustMarshal(t, CertBag{ID: OIDX509Certificate, Value: wrap(cert.der)}))}, "asn1: "},
		{"bytes after the certificate's OCTET STRING", SafeBag{ID: OIDCertBag, Value: wrap(mustMarshal(t, CertBag{ID: OIDX509Certificate, Value: wrap(append(mustMarshal(t, cert.der), 5, 0))}))}, "2 trailing bytes"},
	} {
		t.Run(row.name, func(t *testing.T) {
			der, err := row.bag.Certificate()
			requirePrefix(t, err, row.wantErr)
			require.Nil(t, der)
		})
	}
}

func TestSafeBag_PrivateKey(t *testing.T) {
	cert := newTestCert(t)
	der := cert.pkcs8(t)
	p := pw(t, testPassword)

	t.Run("a keyBag", func(t *testing.T) {
		got, err := SafeBag{ID: OIDKeyBag, Value: wrap(der)}.PrivateKey(Password{})
		require.NoError(t, err)
		require.Equal(t, der, got)
	})

	t.Run("a pkcs8ShroudedKeyBag", func(t *testing.T) {
		got, err := shroud(t, der, testPassword).PrivateKey(p)
		require.NoError(t, err)
		require.Equal(t, der, got)
	})

	t.Run("a pkcs8ShroudedKeyBag whose key is BER", func(t *testing.T) {
		got, err := shroud(t, indefinite(der), testPassword).PrivateKey(p)
		require.NoError(t, err)
		require.Equal(t, der, got, "the decrypted key is rewritten to DER")
	})

	for _, row := range []struct {
		name    string
		bag     SafeBag
		wantErr string
	}{
		{"a keyBag with bytes after the key", SafeBag{ID: OIDKeyBag, Value: wrap(append(append([]byte{}, der...), 5, 0))}, "2 trailing bytes"},
		{"a pkcs8ShroudedKeyBag that isn't one", SafeBag{ID: OIDPKCS8ShroudedKeyBag, Value: wrap(der)}, "asn1: "},
		{"a pkcs8ShroudedKeyBag with RC4", SafeBag{ID: OIDPKCS8ShroudedKeyBag, Value: wrap(mustMarshal(t, EncryptedPrivateKeyInfo{
			Algorithm: algorithm{Algorithm: OIDPBEWithSHAAnd40BitRC4}, EncryptedData: []byte{1},
		}))}, "unsupported encryption algorithm pbeWithSHAAnd40BitRC4 (1.2.840.113549.1.12.1.2)"},
		{"a pkcs8ShroudedKeyBag with bytes after the key", shroud(t, append(append([]byte{}, der...), 5, 0), testPassword), "2 trailing bytes"},
		{"a pkcs8ShroudedKeyBag with a wrong password", shroud(t, der, "wrong"), ""},
	} {
		t.Run(row.name, func(t *testing.T) {
			got, err := row.bag.PrivateKey(p)
			requirePrefix(t, err, row.wantErr)
			require.Nil(t, got)
		})
	}
}

func TestPBMAC1_verifies_with_the_password_as_UTF_8(t *testing.T) {
	safe := authSafeOf(t)
	params := PBKDF2Params{Salt: octetSalt([]byte("saltsalt")), IterationCount: 2048, KeyLength: 32, PRF: algorithm{Algorithm: OIDHMACWithSHA256}}
	mac := MacData{Mac: DigestInfo{Algorithm: algorithm{Algorithm: OIDPBMAC1, Parameters: raw(mustMarshal(t, PBMAC1Params{
		KeyDerivationFunc: pbkdf2Alg(t, params),
		MessageAuthScheme: algorithm{Algorithm: OIDHMACWithSHA256},
	}))}}, Iterations: 1}
	k := pbkdf2Key{salt: []byte("saltsalt"), iterations: 2048, prf: sha256.New}
	key, err := k.derive([]byte("pä密"), 32)
	require.NoError(t, err)
	h := hmac.New(sha256.New, key)
	h.Write(safe)
	mac.Mac.Digest = h.Sum(nil)

	_, _, err = Decode(pfxOf(t, safe, mac), "pä密")
	require.NoError(t, err)
	_, _, err = Decode(pfxOf(t, safe, mac), "pa密")
	require.ErrorIs(t, err, ErrMAC)
}
