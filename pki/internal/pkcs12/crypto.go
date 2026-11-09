// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package pkcs12

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/des"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509/pkix"
	"encoding/asn1"
	"errors"
	"fmt"
	"hash"
)

// algorithm is an AlgorithmIdentifier.
type algorithm = pkix.AlgorithmIdentifier

// hashes maps the object identifiers of a family of algorithms to hashes.
type hashes []struct {
	oid asn1.ObjectIdentifier
	h   func() hash.Hash
}

// digests are the hashes of a MAC that RFC 7292's derivation keys.
var digests = hashes{
	{OIDSHA1, sha1.New},
	{OIDSHA256, sha256.New},
	{OIDSHA384, sha512.New384},
	{OIDSHA512, sha512.New},
}

// hmacs are the HMACs that PBKDF2's PRF and PBMAC1's MAC may be.
var hmacs = hashes{
	{OIDHMACWithSHA1, sha1.New},
	{OIDHMACWithSHA256, sha256.New},
	{OIDHMACWithSHA384, sha512.New384},
	{OIDHMACWithSHA512, sha512.New},
}

// hmacSHA1 is the PRF of PBKDF2 when its parameters name none.
var hmacSHA1 = sha1.New

// lookup returns the hash that hs maps oid to.
func lookup(hs hashes, oid asn1.ObjectIdentifier) (func() hash.Hash, bool) {
	for _, e := range hs {
		if e.oid.Equal(oid) {
			return e.h, true
		}
	}
	return nil, false
}

// unsupported is the error for an algorithm the format doesn't decode.
func unsupported(what string, oid asn1.ObjectIdentifier) error {
	return fmt.Errorf("unsupported %s %s", what, Name(oid))
}

// ErrMAC is wrapped by the error Decode returns when a file's MAC doesn't
// verify with the password.
var ErrMAC = errors.New("the MAC doesn't verify with the password")

// verify reports whether m is the MAC of message with pw.
func (m MacData) verify(message []byte, pw Password) (bool, error) {
	h, key, err := m.key(pw)
	if err != nil {
		return false, err
	}
	mac := hmac.New(h, key)
	mac.Write(message)
	return hmac.Equal(mac.Sum(nil), m.Mac.Digest), nil
}

// key returns the hash of m's HMAC and its key, derived from pw.
func (m MacData) key(pw Password) (func() hash.Hash, []byte, error) {
	alg := m.Mac.Algorithm
	if alg.Algorithm.Equal(OIDPBMAC1) {
		return pbmac1Key(alg.Parameters.FullBytes, pw)
	}
	h, ok := lookup(digests, alg.Algorithm)
	if !ok {
		return nil, nil, unsupported("MAC algorithm", alg.Algorithm)
	}
	if err := checkIterations(m.Iterations); err != nil {
		return nil, nil, err
	}
	return h, Derive(h, pw.bmp, m.MacSalt, m.Iterations, purposeMAC, h().Size()), nil
}

// The bounds of a PBMAC1 key length. Below the lower, a forged file
// verifies about 1 time in 256: OpenSSL's CVE-2026-34181. Above the upper,
// OpenSSL overflowed a buffer: CVE-2025-11187.
const (
	minPBMAC1KeyLength = 20
	maxPBMAC1KeyLength = 64
)

// pbmac1Key decodes PBMAC1's parameters, and returns the hash of its HMAC
// and its key, derived with PBKDF2 from pw as UTF-8.
func pbmac1Key(params []byte, pw Password) (func() hash.Hash, []byte, error) {
	var p PBMAC1Params
	if err := unmarshal(params, &p); err != nil {
		return nil, nil, fmt.Errorf("PBMAC1 parameters: %w", err)
	}
	kdf, err := pbkdf2Params(p.KeyDerivationFunc)
	if err != nil {
		return nil, nil, fmt.Errorf("PBMAC1: %w", err)
	}
	if kdf.keyLength == 0 {
		return nil, nil, errors.New("PBMAC1: PBKDF2 has no key length")
	}
	if kdf.keyLength < minPBMAC1KeyLength || kdf.keyLength > maxPBMAC1KeyLength {
		return nil, nil, fmt.Errorf("PBMAC1: PBKDF2 key length %d, outside %d to %d", kdf.keyLength, minPBMAC1KeyLength, maxPBMAC1KeyLength)
	}
	h, ok := lookup(hmacs, p.MessageAuthScheme.Algorithm)
	if !ok {
		return nil, nil, unsupported("PBMAC1 MAC", p.MessageAuthScheme.Algorithm)
	}
	key, err := kdf.derive(pw.utf8, kdf.keyLength)
	return h, key, err
}

// aesKeySizes are the AES-CBC schemes of PBES2 and their key sizes.
var aesKeySizes = []struct {
	oid  asn1.ObjectIdentifier
	size int
}{
	{OIDAES128CBC, 16},
	{OIDAES192CBC, 24},
	{OIDAES256CBC, 32},
}

// Decrypt decrypts data, which alg encrypted, with pw, and removes its
// padding. alg is PBES2 with PBKDF2 and AES-CBC, or one of RFC 7292's PBE
// schemes with SHA-1 and 3DES or RC2.
func Decrypt(alg pkix.AlgorithmIdentifier, pw Password, data []byte) ([]byte, error) {
	newBlock, key, iv, err := scheme(alg, pw)
	if err != nil {
		return nil, err
	}
	return decryptCBC(newBlock, key, iv, data)
}

// pbeSchemes are the PBE schemes of RFC 7292's Appendix C that the format
// decrypts, with their key sizes in bytes. RC2's effective key length is
// the length of its key. Each has a block of 8 bytes, the size of its IV.
var pbeSchemes = []struct {
	oid      asn1.ObjectIdentifier
	name     string
	keySize  int
	newBlock func([]byte) (cipher.Block, error)
}{
	{OIDPBEWithSHAAnd3KeyTripleDESCBC, "pbeWithSHAAnd3-KeyTripleDES-CBC", 24, des.NewTripleDESCipher},
	{OIDPBEWithSHAAnd128BitRC2CBC, "pbeWithSHAAnd128BitRC2-CBC", 16, rc2Cipher(128)},
	{OIDPBEWithSHAAnd40BitRC2CBC, "pbeWithSHAAnd40BitRC2-CBC", 5, rc2Cipher(40)},
}

// scheme returns the block cipher alg names, and its key and IV, derived
// from pw.
func scheme(alg algorithm, pw Password) (func([]byte) (cipher.Block, error), []byte, []byte, error) {
	if alg.Algorithm.Equal(OIDPBES2) {
		return pbes2(alg.Parameters.FullBytes, pw)
	}
	for _, s := range pbeSchemes {
		if !s.oid.Equal(alg.Algorithm) {
			continue
		}
		var p PBEParams
		if err := unmarshal(alg.Parameters.FullBytes, &p); err != nil {
			return nil, nil, nil, fmt.Errorf("%s parameters: %w", s.name, err)
		}
		if err := checkIterations(p.Iterations); err != nil {
			return nil, nil, nil, fmt.Errorf("%s: %w", s.name, err)
		}
		key := Derive(sha1.New, pw.bmp, p.Salt, p.Iterations, purposeKey, s.keySize)
		iv := Derive(sha1.New, pw.bmp, p.Salt, p.Iterations, purposeIV, des.BlockSize)
		return s.newBlock, key, iv, nil
	}
	return nil, nil, nil, unsupported("encryption algorithm", alg.Algorithm)
}

// pbes2 decodes PBES2's parameters, and returns its AES cipher and its key,
// derived with PBKDF2 from pw as UTF-8, and its IV.
func pbes2(params []byte, pw Password) (func([]byte) (cipher.Block, error), []byte, []byte, error) {
	var p PBES2Params
	if err := unmarshal(params, &p); err != nil {
		return nil, nil, nil, fmt.Errorf("PBES2 parameters: %w", err)
	}
	kdf, err := pbkdf2Params(p.KeyDerivationFunc)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("PBES2: %w", err)
	}
	size := 0
	for _, s := range aesKeySizes {
		if s.oid.Equal(p.EncryptionScheme.Algorithm) {
			size = s.size
		}
	}
	if size == 0 {
		return nil, nil, nil, unsupported("PBES2 encryption scheme", p.EncryptionScheme.Algorithm)
	}
	if kdf.keyLength != 0 && kdf.keyLength != size {
		return nil, nil, nil, fmt.Errorf("PBES2: PBKDF2 key length %d, not the %d bytes of %s", kdf.keyLength, size, Name(p.EncryptionScheme.Algorithm))
	}
	var iv []byte
	if err := unmarshal(p.EncryptionScheme.Parameters.FullBytes, &iv); err != nil {
		return nil, nil, nil, fmt.Errorf("PBES2: %s IV: %w", Name(p.EncryptionScheme.Algorithm), err)
	}
	key, err := kdf.derive(pw.utf8, size)
	return aes.NewCipher, key, iv, err
}

// decryptCBC decrypts data in CBC mode with the cipher newBlock makes from
// key, and iv, and removes its PKCS#7 padding.
func decryptCBC(newBlock func([]byte) (cipher.Block, error), key, iv, data []byte) ([]byte, error) {
	block, err := newBlock(key)
	if err != nil {
		return nil, err
	}
	size := block.BlockSize()
	if len(iv) != size {
		return nil, fmt.Errorf("an IV of %d bytes, not %d", len(iv), size)
	}
	if len(data) == 0 || len(data)%size != 0 {
		return nil, fmt.Errorf("%d bytes of ciphertext, not a positive multiple of %d", len(data), size)
	}
	out := make([]byte, len(data))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(out, data)
	n := int(out[len(out)-1])
	if n == 0 || n > size {
		return nil, errors.New("padding that isn't valid PKCS#7")
	}
	for _, c := range out[len(out)-n:] {
		if int(c) != n {
			return nil, errors.New("padding that isn't valid PKCS#7")
		}
	}
	return out[:len(out)-n], nil
}
