// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package pkcs12

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/asn1"
	"fmt"
	"io"
	"slices"
)

// The parameters of the encoder: those of go-pkcs12's Modern2023, which
// are OpenSSL 3's defaults.
const (
	// EncodeIterations is the iteration count of every key derivation the
	// encoder writes: PBKDF2's for the certificates and the key, and RFC
	// 7292's for the MAC.
	EncodeIterations = 2048

	// EncodeSaltSize is the size in bytes of every salt the encoder
	// writes.
	EncodeSaltSize = 16
)

// The object identifiers that only the encoder writes.
var (
	// OIDLocalKeyID is PKCS#9's localKeyId attribute, which pairs a key
	// with its certificate.
	OIDLocalKeyID = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 21}

	// OIDAnyExtendedKeyUsage is X.509's anyExtendedKeyUsage, the value of
	// Java's trust attribute.
	OIDAnyExtendedKeyUsage = asn1.ObjectIdentifier{2, 5, 29, 37, 0}
)

// Encode returns a PFX in DER that holds key, the DER of a PKCS#8 private
// key, and certs, the DER of its chain, leaf first, protected with
// password. It lays the file out as go-pkcs12's Encode does: the
// certificates in one encryptedData ContentInfo, in the order of certs,
// and the key in a pkcs8ShroudedKeyBag in a data ContentInfo, the key and
// the leaf each with a localKeyId attribute, the SHA-1 of the leaf. Both
// are encrypted with PBES2, PBKDF2-HMAC-SHA-256 and AES-256-CBC, and the
// MAC is HMAC-SHA-256, keyed by RFC 7292's derivation.
//
// rand is the source of the salts and the IVs. Encode fails if reading it
// fails, or if password isn't valid UTF-8 or holds a character outside
// the Basic Multilingual Plane. It panics if certs is empty.
func Encode(rand io.Reader, key []byte, certs [][]byte, password string) ([]byte, error) {
	pw, err := encodePassword(password)
	if err != nil {
		return nil, err
	}
	fingerprint := sha1.Sum(certs[0])
	localKeyID := attribute(OIDLocalKeyID, marshal(fingerprint[:]))
	bags := make([]SafeBag, len(certs))
	for i, der := range certs {
		bags[i] = certBag(der)
	}
	bags[0].Attributes = []Attribute{localKeyID}
	certInfo, err := encryptedContents(rand, pw, bags)
	if err != nil {
		return nil, fmt.Errorf("the certificates: %w", err)
	}
	alg, encrypted, err := encryptPBES2(rand, pw, key)
	if err != nil {
		return nil, fmt.Errorf("the key: %w", err)
	}
	keyBag := SafeBag{
		ID:         OIDPKCS8ShroudedKeyBag,
		Value:      explicit(marshal(EncryptedPrivateKeyInfo{Algorithm: alg, EncryptedData: encrypted})),
		Attributes: []Attribute{localKeyID},
	}
	keyInfo := ContentInfo{ContentType: OIDData, Content: explicit(marshal(marshal([]SafeBag{keyBag})))}
	return encodePFX(rand, pw, certInfo, keyInfo)
}

// EncodeTrustStore returns a PFX in DER that holds certs, the DER of the
// certificates of a trust store, protected with password. It lays the file
// out as go-pkcs12's EncodeTrustStore does: the certificates in one
// encryptedData ContentInfo, in the order of certs, each marked trusted
// with Java's attribute and anyExtendedKeyUsage. It encrypts and computes
// the MAC as Encode does, and fails as Encode does.
func EncodeTrustStore(rand io.Reader, certs [][]byte, password string) ([]byte, error) {
	pw, err := encodePassword(password)
	if err != nil {
		return nil, err
	}
	trusted := attribute(OIDJavaTrustStore, marshal(OIDAnyExtendedKeyUsage))
	bags := make([]SafeBag, len(certs))
	for i, der := range certs {
		bags[i] = certBag(der)
		bags[i].Attributes = []Attribute{trusted}
	}
	certInfo, err := encryptedContents(rand, pw, bags)
	if err != nil {
		return nil, fmt.Errorf("the certificates: %w", err)
	}
	return encodePFX(rand, pw, certInfo)
}

// encodePassword returns the one form of password, as the encoder writes
// it.
func encodePassword(password string) (Password, error) {
	pws, err := passwords(password)
	if err != nil {
		return Password{}, err
	}
	return pws[0], nil
}

// encodePFX returns the DER of a PFX whose AuthenticatedSafe is cis, with
// an HMAC-SHA-256 over it keyed from pw by RFC 7292's derivation.
func encodePFX(rand io.Reader, pw Password, cis ...ContentInfo) ([]byte, error) {
	authSafe := marshal(cis)
	salt, err := random(rand, EncodeSaltSize)
	if err != nil {
		return nil, fmt.Errorf("the MAC: %w", err)
	}
	mac := hmac.New(sha256.New, Derive(sha256.New, pw.bmp, salt, EncodeIterations, purposeMAC, sha256.Size))
	mac.Write(authSafe)
	return marshal(PFX{
		Version:  3,
		AuthSafe: ContentInfo{ContentType: OIDData, Content: explicit(marshal(authSafe))},
		MacData: MacData{
			Mac:        DigestInfo{Algorithm: algorithm{Algorithm: OIDSHA256, Parameters: asn1.NullRawValue}, Digest: mac.Sum(nil)},
			MacSalt:    salt,
			Iterations: EncodeIterations,
		},
	}), nil
}

// encryptedContents returns an encryptedData ContentInfo of a
// SafeContents of bags, encrypted with pw.
func encryptedContents(rand io.Reader, pw Password, bags []SafeBag) (ContentInfo, error) {
	alg, encrypted, err := encryptPBES2(rand, pw, marshal(bags))
	if err != nil {
		return ContentInfo{}, err
	}
	ed := EncryptedData{
		EncryptedContentInfo: EncryptedContentInfo{
			ContentType:                OIDData,
			ContentEncryptionAlgorithm: alg,
			EncryptedContent:           asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, Bytes: encrypted},
		},
	}
	return ContentInfo{ContentType: OIDEncryptedData, Content: explicit(marshal(ed))}, nil
}

// encryptPBES2 encrypts plaintext, padded with PKCS#7, with PBES2: an
// AES-256-CBC key derived from pw as UTF-8 with PBKDF2-HMAC-SHA-256, over
// a new salt, and a new IV. It returns the AlgorithmIdentifier that names
// them, and the ciphertext.
func encryptPBES2(rand io.Reader, pw Password, plaintext []byte) (algorithm, []byte, error) {
	saltAndIV, err := random(rand, EncodeSaltSize+aes.BlockSize)
	if err != nil {
		return algorithm{}, nil, err
	}
	salt, iv := saltAndIV[:EncodeSaltSize], saltAndIV[EncodeSaltSize:]
	kdf := pbkdf2Key{salt: salt, iterations: EncodeIterations, prf: sha256.New}
	block := must(aes.NewCipher(must(kdf.derive(pw.utf8, 32))))
	alg := algorithm{Algorithm: OIDPBES2, Parameters: asn1.RawValue{FullBytes: marshal(PBES2Params{
		KeyDerivationFunc: algorithm{Algorithm: OIDPBKDF2, Parameters: asn1.RawValue{FullBytes: marshal(PBKDF2Params{
			Salt:           asn1.RawValue{Tag: asn1.TagOctetString, Bytes: salt},
			IterationCount: EncodeIterations,
			PRF:            algorithm{Algorithm: OIDHMACWithSHA256, Parameters: asn1.NullRawValue},
		})}},
		EncryptionScheme: algorithm{Algorithm: OIDAES256CBC, Parameters: asn1.RawValue{FullBytes: marshal(iv)}},
	})}}
	n := aes.BlockSize - len(plaintext)%aes.BlockSize
	out := slices.Concat(plaintext, bytes.Repeat([]byte{byte(n)}, n))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(out, out)
	return alg, out, nil
}

// certBag returns a certBag that holds der, an X.509 certificate.
func certBag(der []byte) SafeBag {
	value := marshal(CertBag{ID: OIDX509Certificate, Value: explicit(marshal(der))})
	return SafeBag{ID: OIDCertBag, Value: explicit(value)}
}

// attribute returns the attribute id with one value, the DER value.
func attribute(id asn1.ObjectIdentifier, value []byte) Attribute {
	return Attribute{ID: id, Values: asn1.RawValue{Tag: asn1.TagSet, IsCompound: true, Bytes: value}}
}

// explicit returns der in a [0] EXPLICIT tag.
func explicit(der []byte) asn1.RawValue {
	return asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true, Bytes: der}
}

// random returns n bytes read from rand.
func random(rand io.Reader, n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := io.ReadFull(rand, b); err != nil {
		return nil, fmt.Errorf("reading random bytes: %w", err)
	}
	return b, nil
}

// marshal is asn1.Marshal of v, one of the encoder's own structures,
// which always marshal.
func marshal(v any) []byte {
	return must(asn1.Marshal(v))
}

// must returns v, and panics if err is not nil. The encoder calls it only
// where an error is a broken invariant: marshaling its own structures,
// and deriving and using an AES-256 key of the right size.
func must[T any](v T, err error) T {
	if err != nil {
		panic(fmt.Sprintf("pkcs12: encoder invariant broken: %v", err))
	}
	return v
}
