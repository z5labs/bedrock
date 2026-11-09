// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package pki_test

import (
	"bytes"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/des"
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash"
	"math/bits"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/stretchr/testify/require"
	sslmate "software.sslmate.com/src/go-pkcs12"

	"github.com/z5labs/bedrock/pki"
	"github.com/z5labs/bedrock/pki/internal/pkcs12"
)

// This file re-encodes the tables' files as BER, as NSS, Firefox and
// Bouncy Castle write PKCS#12, and builds the files RC2 encrypts, which
// needs RC2 encryption that pki doesn't have.

// segmentSize is the size of the segments that berOf splits an OCTET
// STRING into.
const segmentSize = 7

// berOf re-encodes der, one DER value, as BER: every constructed value with
// an indefinite length, and every OCTET STRING constructed, of segments of
// segmentSize octets. A primitive value with an implicit [0] tag is taken
// to be an OCTET STRING too, since in a PKCS#12 file it is an
// EncryptedContentInfo's encryptedContent. The content octets of an OCTET
// STRING are kept as they are. It reports whether der was one DER value.
func berOf(der []byte) ([]byte, bool) {
	var v asn1.RawValue
	rest, err := asn1.Unmarshal(der, &v)
	if err != nil || len(rest) > 0 {
		return nil, false
	}
	return berValue(v)
}

func berValue(v asn1.RawValue) ([]byte, bool) {
	octetString := !v.IsCompound && (v.Class == asn1.ClassUniversal && v.Tag == asn1.TagOctetString ||
		v.Class == asn1.ClassContextSpecific && v.Tag == 0)
	switch {
	case v.IsCompound:
		out := append(identifierOf(v.FullBytes), 0x80)
		for rest := v.Bytes; len(rest) > 0; {
			var child asn1.RawValue
			var err error
			if rest, err = asn1.Unmarshal(rest, &child); err != nil {
				return nil, false
			}
			ber, ok := berValue(child)
			if !ok {
				return nil, false
			}
			out = append(out, ber...)
		}
		return append(out, 0, 0), true
	case octetString:
		out := append(identifierOf(v.FullBytes), 0x80)
		out[0] |= 0x20
		for i := 0; i < len(v.Bytes); i += segmentSize {
			segment := v.Bytes[i:min(i+segmentSize, len(v.Bytes))]
			out = append(append(out, asn1.TagOctetString, byte(len(segment))), segment...)
		}
		return append(out, 0, 0), true
	default:
		return v.FullBytes, true
	}
}

// identifierOf returns the identifier octets of der.
func identifierOf(der []byte) []byte {
	n := 1
	if der[0]&0x1f == 0x1f {
		for der[n]&0x80 != 0 {
			n++
		}
		n++
	}
	return append([]byte{}, der[:n]...)
}

// mustBER is berOf, failing the test if der isn't one DER value.
func mustBER(tb testing.TB, der []byte) []byte {
	tb.Helper()
	ber, ok := berOf(der)
	require.True(tb, ok, "one DER value")
	return ber
}

// unmarshalAll is asn1.Unmarshal of der, which must be exactly one value.
func unmarshalAll(tb testing.TB, der []byte, out any) {
	tb.Helper()
	rest, err := asn1.Unmarshal(der, out)
	require.NoError(tb, err)
	require.Empty(tb, rest)
}

// berFile re-encodes data, a DER file of password, as BER at every level:
// the PFX, with the authSafe's content split into segments, the
// AuthenticatedSafe, each SafeContents, decrypted and encrypted again, and
// each private key, decrypted and encrypted again. It re-computes the MAC
// over the content octets of the new AuthenticatedSafe.
func berFile(tb testing.TB, data []byte, password string) []byte {
	tb.Helper()
	// The tests decrypt with the form of password that pki verified.
	_, pw, err := pkcs12.Decode(data, password)
	require.NoError(tb, err)
	decrypt := func(alg pkix.AlgorithmIdentifier, ciphertext []byte) []byte {
		plaintext, err := pkcs12.Decrypt(alg, pw, ciphertext)
		require.NoError(tb, err)
		return plaintext
	}
	var p pkcs12.PFX
	unmarshalAll(tb, data, &p)
	var authSafe []byte
	unmarshalAll(tb, p.AuthSafe.Content.Bytes, &authSafe)
	var cis []pkcs12.ContentInfo
	unmarshalAll(tb, authSafe, &cis)
	for i, ci := range cis {
		if ci.ContentType.Equal(pkcs12.OIDData) {
			var contents []byte
			unmarshalAll(tb, ci.Content.Bytes, &contents)
			cis[i].Content = explicit(marshal(tb, berSafeContents(tb, contents, password, decrypt)))
			continue
		}
		var ed pkcs12.EncryptedData
		unmarshalAll(tb, ci.Content.Bytes, &ed)
		info := &ed.EncryptedContentInfo
		contents := decrypt(info.ContentEncryptionAlgorithm, info.EncryptedContent.Bytes)
		info.EncryptedContent = implicitOctets(encryptPBE(tb, info.ContentEncryptionAlgorithm, password, berSafeContents(tb, contents, password, decrypt)))
		cis[i].Content = explicit(marshal(tb, ed))
	}
	ber := mustBER(tb, marshal(tb, cis))
	p.AuthSafe.Content = explicit(marshal(tb, ber))
	p.MacData = remac(tb, p.MacData, ber, password)
	return mustBER(tb, marshal(tb, p))
}

// berSafeContents re-encodes contents, a DER SafeContents, as BER, with
// each pkcs8ShroudedKeyBag's key decrypted, re-encoded as BER and
// encrypted again with password.
func berSafeContents(tb testing.TB, contents []byte, password string, decrypt func(pkix.AlgorithmIdentifier, []byte) []byte) []byte {
	tb.Helper()
	var bags []pkcs12.SafeBag
	unmarshalAll(tb, contents, &bags)
	for i, bag := range bags {
		if !bag.ID.Equal(pkcs12.OIDPKCS8ShroudedKeyBag) {
			continue
		}
		var info pkcs12.EncryptedPrivateKeyInfo
		unmarshalAll(tb, bag.Value.Bytes, &info)
		key := mustBER(tb, decrypt(info.Algorithm, info.EncryptedData))
		info.EncryptedData = encryptPBE(tb, info.Algorithm, password, key)
		bags[i].Value = explicit(marshal(tb, info))
	}
	return mustBER(tb, marshal(tb, bags))
}

// implicitOctets is b as an OCTET STRING with an implicit [0] tag.
func implicitOctets(b []byte) asn1.RawValue {
	return asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, Bytes: b}
}

// bmpString encodes password as RFC 7292's BMPString, with its terminator.
func bmpString(password string) []byte {
	out := []byte{}
	for _, u := range utf16.Encode([]rune(password)) {
		out = append(out, byte(u>>8), byte(u))
	}
	return append(out, 0, 0)
}

// hashOf returns the hash that oid names, a digest or an HMAC.
func hashOf(tb testing.TB, oid asn1.ObjectIdentifier) func() hash.Hash {
	tb.Helper()
	for _, h := range []struct {
		oids []asn1.ObjectIdentifier
		h    func() hash.Hash
	}{
		{[]asn1.ObjectIdentifier{pkcs12.OIDSHA1, pkcs12.OIDHMACWithSHA1, nil}, sha1.New},
		{[]asn1.ObjectIdentifier{pkcs12.OIDSHA256, pkcs12.OIDHMACWithSHA256}, sha256.New},
		{[]asn1.ObjectIdentifier{pkcs12.OIDSHA384, pkcs12.OIDHMACWithSHA384}, sha512.New384},
		{[]asn1.ObjectIdentifier{pkcs12.OIDSHA512, pkcs12.OIDHMACWithSHA512}, sha512.New},
	} {
		for _, o := range h.oids {
			if o.Equal(oid) {
				return h.h
			}
		}
	}
	require.FailNow(tb, "an unknown hash", "%s", oid)
	return nil
}

// remac returns mac re-computed over authSafe with password. A file with
// no MAC keeps none.
func remac(tb testing.TB, mac pkcs12.MacData, authSafe []byte, password string) pkcs12.MacData {
	tb.Helper()
	alg := mac.Mac.Algorithm
	var h func() hash.Hash
	var key []byte
	switch {
	case len(alg.Algorithm) == 0:
		return mac
	case alg.Algorithm.Equal(pkcs12.OIDPBMAC1):
		var params pkcs12.PBMAC1Params
		unmarshalAll(tb, alg.Parameters.FullBytes, &params)
		var kdf pkcs12.PBKDF2Params
		unmarshalAll(tb, params.KeyDerivationFunc.Parameters.FullBytes, &kdf)
		var err error
		key, err = pbkdf2.Key(hashOf(tb, kdf.PRF.Algorithm), password, kdf.Salt.Bytes, kdf.IterationCount, kdf.KeyLength)
		require.NoError(tb, err)
		h = hashOf(tb, params.MessageAuthScheme.Algorithm)
	default:
		h = hashOf(tb, alg.Algorithm)
		key = pkcs12.Derive(h, bmpString(password), mac.MacSalt, mac.Iterations, 3, h().Size())
	}
	m := hmac.New(h, key)
	m.Write(authSafe)
	mac.Mac.Digest = m.Sum(nil)
	return mac
}

// pbeCipher returns the block cipher and IV with which alg encrypts with
// password, for the algorithms of the tables' files.
func pbeCipher(tb testing.TB, alg pkix.AlgorithmIdentifier, password string) (cipher.Block, []byte) {
	tb.Helper()
	if alg.Algorithm.Equal(pkcs12.OIDPBES2) {
		var params pkcs12.PBES2Params
		unmarshalAll(tb, alg.Parameters.FullBytes, &params)
		var kdf pkcs12.PBKDF2Params
		unmarshalAll(tb, params.KeyDerivationFunc.Parameters.FullBytes, &kdf)
		size := map[string]int{
			pkcs12.OIDAES128CBC.String(): 16,
			pkcs12.OIDAES192CBC.String(): 24,
			pkcs12.OIDAES256CBC.String(): 32,
		}[params.EncryptionScheme.Algorithm.String()]
		key, err := pbkdf2.Key(hashOf(tb, kdf.PRF.Algorithm), password, kdf.Salt.Bytes, kdf.IterationCount, size)
		require.NoError(tb, err)
		block, err := aes.NewCipher(key)
		require.NoError(tb, err)
		var iv []byte
		unmarshalAll(tb, params.EncryptionScheme.Parameters.FullBytes, &iv)
		return block, iv
	}
	var params pkcs12.PBEParams
	unmarshalAll(tb, alg.Parameters.FullBytes, &params)
	derive := func(id byte, size int) []byte {
		return pkcs12.Derive(sha1.New, bmpString(password), params.Salt, params.Iterations, id, size)
	}
	iv := derive(2, 8)
	switch {
	case alg.Algorithm.Equal(pkcs12.OIDPBEWithSHAAnd3KeyTripleDESCBC):
		block, err := des.NewTripleDESCipher(derive(1, 24))
		require.NoError(tb, err)
		return block, iv
	case alg.Algorithm.Equal(pkcs12.OIDPBEWithSHAAnd128BitRC2CBC):
		return newTestRC2(derive(1, 16), 128), iv
	case alg.Algorithm.Equal(pkcs12.OIDPBEWithSHAAnd40BitRC2CBC):
		return newTestRC2(derive(1, 5), 40), iv
	}
	require.FailNow(tb, "an unknown encryption algorithm", "%s", alg.Algorithm)
	return nil, nil
}

// encryptPBE encrypts plaintext, padded, as alg names it, with password.
func encryptPBE(tb testing.TB, alg pkix.AlgorithmIdentifier, password string, plaintext []byte) []byte {
	tb.Helper()
	block, iv := pbeCipher(tb, alg, password)
	n := block.BlockSize() - len(plaintext)%block.BlockSize()
	out := append(append([]byte{}, plaintext...), bytes.Repeat([]byte{byte(n)}, n)...)
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(out, out)
	return out
}

// testRC2 is RC2 of RFC 2268, written apart from pki's so that the files it
// encrypts check pki's decryption rather than agree with it.
type testRC2 struct{ k [64]uint16 }

// testPITable is PITABLE of RFC 2268's section 2.
var testPITable = [256]byte{
	0xd9, 0x78, 0xf9, 0xc4, 0x19, 0xdd, 0xb5, 0xed, 0x28, 0xe9, 0xfd, 0x79, 0x4a, 0xa0, 0xd8, 0x9d,
	0xc6, 0x7e, 0x37, 0x83, 0x2b, 0x76, 0x53, 0x8e, 0x62, 0x4c, 0x64, 0x88, 0x44, 0x8b, 0xfb, 0xa2,
	0x17, 0x9a, 0x59, 0xf5, 0x87, 0xb3, 0x4f, 0x13, 0x61, 0x45, 0x6d, 0x8d, 0x09, 0x81, 0x7d, 0x32,
	0xbd, 0x8f, 0x40, 0xeb, 0x86, 0xb7, 0x7b, 0x0b, 0xf0, 0x95, 0x21, 0x22, 0x5c, 0x6b, 0x4e, 0x82,
	0x54, 0xd6, 0x65, 0x93, 0xce, 0x60, 0xb2, 0x1c, 0x73, 0x56, 0xc0, 0x14, 0xa7, 0x8c, 0xf1, 0xdc,
	0x12, 0x75, 0xca, 0x1f, 0x3b, 0xbe, 0xe4, 0xd1, 0x42, 0x3d, 0xd4, 0x30, 0xa3, 0x3c, 0xb6, 0x26,
	0x6f, 0xbf, 0x0e, 0xda, 0x46, 0x69, 0x07, 0x57, 0x27, 0xf2, 0x1d, 0x9b, 0xbc, 0x94, 0x43, 0x03,
	0xf8, 0x11, 0xc7, 0xf6, 0x90, 0xef, 0x3e, 0xe7, 0x06, 0xc3, 0xd5, 0x2f, 0xc8, 0x66, 0x1e, 0xd7,
	0x08, 0xe8, 0xea, 0xde, 0x80, 0x52, 0xee, 0xf7, 0x84, 0xaa, 0x72, 0xac, 0x35, 0x4d, 0x6a, 0x2a,
	0x96, 0x1a, 0xd2, 0x71, 0x5a, 0x15, 0x49, 0x74, 0x4b, 0x9f, 0xd0, 0x5e, 0x04, 0x18, 0xa4, 0xec,
	0xc2, 0xe0, 0x41, 0x6e, 0x0f, 0x51, 0xcb, 0xcc, 0x24, 0x91, 0xaf, 0x50, 0xa1, 0xf4, 0x70, 0x39,
	0x99, 0x7c, 0x3a, 0x85, 0x23, 0xb8, 0xb4, 0x7a, 0xfc, 0x02, 0x36, 0x5b, 0x25, 0x55, 0x97, 0x31,
	0x2d, 0x5d, 0xfa, 0x98, 0xe3, 0x8a, 0x92, 0xae, 0x05, 0xdf, 0x29, 0x10, 0x67, 0x6c, 0xba, 0xc9,
	0xd3, 0x00, 0xe6, 0xcf, 0xe1, 0x9e, 0xa8, 0x2c, 0x63, 0x16, 0x01, 0x3f, 0x58, 0xe2, 0x89, 0xa9,
	0x0d, 0x38, 0x34, 0x1b, 0xab, 0x33, 0xff, 0xb0, 0xbb, 0x48, 0x0c, 0x5f, 0xb9, 0xb1, 0xcd, 0x2e,
	0xc5, 0xf3, 0xdb, 0x47, 0xe5, 0xa5, 0x9c, 0x77, 0x0a, 0xa6, 0x20, 0x68, 0xfe, 0x7f, 0xc1, 0xad,
}

func newTestRC2(key []byte, effective int) *testRC2 {
	l := make([]byte, 128)
	copy(l, key)
	for i := len(key); i < 128; i++ {
		l[i] = testPITable[byte(int(l[i-1])+int(l[i-len(key)]))]
	}
	t8 := (effective + 7) / 8
	l[128-t8] = testPITable[l[128-t8]&byte(255%(int(1)<<(8+effective-8*t8)))]
	for i := 127 - t8; i >= 0; i-- {
		l[i] = testPITable[l[i+1]^l[i+t8]]
	}
	c := &testRC2{}
	for i := range c.k {
		c.k[i] = binary.LittleEndian.Uint16(l[2*i:])
	}
	return c
}

func (*testRC2) BlockSize() int { return 8 }

// Encrypt is RFC 2268's section 3.
func (c *testRC2) Encrypt(dst, src []byte) {
	var r [4]uint16
	for i := range r {
		r[i] = binary.LittleEndian.Uint16(src[2*i:])
	}
	shifts := [4]int{1, 2, 3, 5}
	j := 0
	for round := range 16 {
		for i := range 4 {
			r[i] += c.k[j] + (r[(i+3)%4] & r[(i+2)%4]) + (^r[(i+3)%4] & r[(i+1)%4])
			r[i] = bits.RotateLeft16(r[i], shifts[i])
			j++
		}
		if round == 4 || round == 10 {
			for i := range 4 {
				r[i] += c.k[r[(i+3)%4]&63]
			}
		}
	}
	for i := range r {
		binary.LittleEndian.PutUint16(dst[2*i:], r[i])
	}
}

// Decrypt panics: the tests decrypt RC2 with pki's.
func (*testRC2) Decrypt(dst, src []byte) {
	panic("testRC2 only encrypts")
}

func TestTestRC2_encrypts_RFC_2268s_test_vectors(t *testing.T) {
	for _, row := range []struct {
		key        string
		effective  int
		plaintext  string
		ciphertext string
	}{
		{"0000000000000000", 63, "0000000000000000", "ebb773f993278eff"},
		{"ffffffffffffffff", 64, "ffffffffffffffff", "278b27e42e2f0d49"},
		{"3000000000000000", 64, "1000000000000001", "30649edf9be7d2c2"},
		{"88", 64, "0000000000000000", "61a8a244adacccf0"},
		{"88bca90e90875a", 64, "0000000000000000", "6ccf4308974c267f"},
		{"88bca90e90875a7f0f79c384627bafb2", 64, "0000000000000000", "1a807d272bbe5db1"},
		{"88bca90e90875a7f0f79c384627bafb2", 128, "0000000000000000", "2269552ab0f85ca6"},
		{"88bca90e90875a7f0f79c384627bafb216f80a6f85920584c42fceb0be255daf1e", 129, "0000000000000000", "5b78d3a43dfff1f1"},
	} {
		key, err := hex.DecodeString(row.key)
		require.NoError(t, err)
		plaintext, err := hex.DecodeString(row.plaintext)
		require.NoError(t, err)
		got := make([]byte, 8)
		newTestRC2(key, row.effective).Encrypt(got, plaintext)
		require.Equal(t, row.ciphertext, hex.EncodeToString(got))
	}
}

// rc2File returns a file of c, with password, whose certificates and key
// are both encrypted with pbeWithSHAAnd128BitRC2-CBC, which go-pkcs12
// doesn't write, and whose MAC is HMAC-SHA-1.
func rc2File(tb testing.TB, c chain, password string) []byte {
	tb.Helper()
	pbe := func() pkix.AlgorithmIdentifier {
		params := pkcs12.PBEParams{Salt: []byte(rand.Text()[:8]), Iterations: 2048}
		return pkix.AlgorithmIdentifier{Algorithm: pkcs12.OIDPBEWithSHAAnd128BitRC2CBC, Parameters: asn1.RawValue{FullBytes: marshal(tb, params)}}
	}
	var certs []pkcs12.SafeBag
	for _, cert := range c.certs {
		certs = append(certs, certBag(tb, cert.Raw))
	}
	certAlg := pbe()
	ed := pkcs12.EncryptedData{EncryptedContentInfo: pkcs12.EncryptedContentInfo{
		ContentType:                pkcs12.OIDData,
		ContentEncryptionAlgorithm: certAlg,
		EncryptedContent:           implicitOctets(encryptPBE(tb, certAlg, password, marshal(tb, certs))),
	}}
	der, err := x509.MarshalPKCS8PrivateKey(c.key)
	require.NoError(tb, err)
	keyAlg := pbe()
	key := safeBag(pkcs12.OIDPKCS8ShroudedKeyBag, marshal(tb, pkcs12.EncryptedPrivateKeyInfo{Algorithm: keyAlg, EncryptedData: encryptPBE(tb, keyAlg, password, der)}))
	safe := marshal(tb, []pkcs12.ContentInfo{
		{ContentType: pkcs12.OIDEncryptedData, Content: explicit(marshal(tb, ed))},
		{ContentType: pkcs12.OIDData, Content: explicit(marshal(tb, marshal(tb, []pkcs12.SafeBag{key})))},
	})
	mac := pkcs12.MacData{
		Mac:        pkcs12.DigestInfo{Algorithm: pkix.AlgorithmIdentifier{Algorithm: pkcs12.OIDSHA1, Parameters: asn1.NullRawValue}},
		MacSalt:    []byte(rand.Text()[:8]),
		Iterations: 2048,
	}
	return pfx(tb, safe, remac(tb, mac, safe, password))
}

func TestPKCS12KeyPair_decrypts_128_bit_RC2(t *testing.T) {
	m := newMaterial(t)
	for _, c := range chains(t, m) {
		t.Run(c.name, func(t *testing.T) {
			pair, err := pki.PKCS12KeyPair(rc2File(t, c, password), password)
			require.NoError(t, err)
			want := [][]byte{}
			for _, cert := range c.certs {
				want = append(want, cert.Raw)
			}
			require.Equal(t, want, pair.Certificate)
			require.Equal(t, c.leaf(), pair.Leaf)
			key, ok := c.key.(interface{ Equal(crypto.PrivateKey) bool })
			require.True(t, ok)
			require.True(t, key.Equal(pair.PrivateKey))
		})
	}
}

func TestPKCS12_decodes_BER_to_what_its_DER_decodes_to(t *testing.T) {
	m := newMaterial(t)
	for _, row := range validPKCS12Pairs(t, m) {
		t.Run(row.name, func(t *testing.T) {
			want, err := pki.PKCS12KeyPair(row.data, row.password)
			require.NoError(t, err)

			ber := berFile(t, row.data, row.password)
			require.NotEqual(t, row.data, ber)
			got, err := pki.PKCS12KeyPair(ber, row.password)
			require.NoError(t, err)
			require.Equal(t, want, got)
		})
	}

	for _, row := range validTrustStores(t, m) {
		t.Run("a trust store, "+row.name, func(t *testing.T) {
			want, err := pki.PKCS12Pool(row.data, row.password)
			require.NoError(t, err)

			got, err := pki.PKCS12Pool(berFile(t, row.data, row.password), row.password)
			require.NoError(t, err)
			require.True(t, want.Equal(got), "the pools are equal")
		})
	}

	t.Run("128-bit RC2", func(t *testing.T) {
		data := rc2File(t, chains(t, m)[0], password)
		want, err := pki.PKCS12KeyPair(data, password)
		require.NoError(t, err)

		got, err := pki.PKCS12KeyPair(berFile(t, data, password), password)
		require.NoError(t, err)
		require.Equal(t, want, got)
	})
}

func TestPKCS12_rejects_BER_that_DER_forbids_otherwise(t *testing.T) {
	m := newMaterial(t)
	ec := chains(t, m)[0]
	data := encode(t, sslmate.Modern2023, ec, password)
	var outer asn1.RawValue
	unmarshalAll(t, data, &outer)
	// The PFX's contents begin with its version, 02 01 03.
	contents := outer.Bytes
	require.Equal(t, []byte{2, 1, 3}, contents[:3])
	eocInside := marshal(t, asn1.RawValue{Tag: asn1.TagSequence, IsCompound: true, Bytes: append([]byte{2, 1, 3, 0, 0}, contents[3:]...)})
	eocOffset := len(eocInside) - len(contents) - 2 + 3
	ber := mustBER(t, data)
	leafBER := mustBER(t, ec.leaf().Raw)

	for _, row := range []struct {
		name    string
		data    []byte
		wantErr string
	}{
		{"a length in more octets than it needs",
			append(append([]byte{0x30, 0x80, 2, 0x81, 1, 3}, contents[3:]...), 0, 0),
			"pki: PKCS#12: PFX: BER at offset 3: a length in more octets than it needs"},
		{"a missing end-of-contents", ber[:len(ber)-2],
			"pki: PKCS#12: PFX: BER at offset 0: a missing end-of-contents"},
		{"an end-of-contents inside a definite length", eocInside,
			fmt.Sprintf("pki: PKCS#12: PFX: BER at offset %d: an end-of-contents inside a definite length", eocOffset)},
		{"nesting deeper than 64", append(append(bytes.Repeat([]byte{0x30, 0x80}, 65), 2, 1, 3), bytes.Repeat([]byte{0, 0}, 65)...),
			"pki: PKCS#12: PFX: BER at offset 128: nesting deeper than 64"},
		{"a BER certificate inside a certificate bag", mustBER(t, sha1MAC(t, password, 2048, []pkcs12.SafeBag{certBag(t, leafBER), keyBag(t, m.ecKey)})),
			"pki: PKCS#12: bag 1: certBag: x509: "},
	} {
		t.Run(row.name, func(t *testing.T) {
			got, err := pki.PKCS12KeyPair(row.data, password)
			require.Error(t, err)
			require.True(t, strings.HasPrefix(err.Error(), row.wantErr), "the error %q starts with %q", err, row.wantErr)
			require.NotErrorIs(t, err, pki.ErrIncorrectPassword)
			require.Equal(t, tls.Certificate{}, got)
		})
	}
}

func TestPKCS12KeyPair_verifies_the_MAC_over_the_octets_as_written(t *testing.T) {
	data := encode(t, sslmate.Modern2023, chains(t, newMaterial(t))[0], password)
	var p pkcs12.PFX
	unmarshalAll(t, data, &p)
	var authSafe []byte
	unmarshalAll(t, p.AuthSafe.Content.Bytes, &authSafe)
	// The AuthenticatedSafe re-encoded as BER, which decodes to the same
	// structures, under the MAC of its DER.
	p.AuthSafe.Content = explicit(marshal(t, mustBER(t, authSafe)))

	got, err := pki.PKCS12KeyPair(marshal(t, p), password)
	require.ErrorIs(t, err, pki.ErrIncorrectPassword)
	require.Equal(t, tls.Certificate{}, got)
}
