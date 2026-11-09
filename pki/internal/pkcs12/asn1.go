// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package pkcs12 is the PKCS#12 format of RFC 7292, as package pki reads
// it: the ASN.1 structures, the key derivation of RFC 7292's Appendix B,
// the password encodings, decryption and MAC verification. Package pki
// walks the bags that Decode returns and builds its results from them.
package pkcs12

import (
	"crypto/x509/pkix"
	"encoding/asn1"
	"fmt"
)

// The object identifiers of RFC 7292, RFC 8018 and RFC 9579 that the
// format names, and those of the algorithms it fails on by name.
var (
	OIDData          = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 1}
	OIDEncryptedData = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 6}

	OIDKeyBag              = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 12, 10, 1, 1}
	OIDPKCS8ShroudedKeyBag = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 12, 10, 1, 2}
	OIDCertBag             = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 12, 10, 1, 3}
	OIDCRLBag              = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 12, 10, 1, 4}
	OIDSecretBag           = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 12, 10, 1, 5}
	OIDSafeContentsBag     = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 12, 10, 1, 6}

	OIDX509Certificate = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 22, 1}

	// OIDJavaTrustStore is the attribute with which Java's keytool marks a
	// certificate of a trust store as trusted.
	OIDJavaTrustStore = asn1.ObjectIdentifier{2, 16, 840, 1, 113894, 746875, 1, 1}

	OIDSHA1   = asn1.ObjectIdentifier{1, 3, 14, 3, 2, 26}
	OIDSHA256 = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 1}
	OIDSHA384 = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 2}
	OIDSHA512 = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 3}

	OIDHMACWithSHA1   = asn1.ObjectIdentifier{1, 2, 840, 113549, 2, 7}
	OIDHMACWithSHA256 = asn1.ObjectIdentifier{1, 2, 840, 113549, 2, 9}
	OIDHMACWithSHA384 = asn1.ObjectIdentifier{1, 2, 840, 113549, 2, 10}
	OIDHMACWithSHA512 = asn1.ObjectIdentifier{1, 2, 840, 113549, 2, 11}

	OIDPBKDF2 = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 5, 12}
	OIDPBES2  = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 5, 13}
	OIDPBMAC1 = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 5, 14}

	OIDAES128CBC = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 1, 2}
	OIDAES192CBC = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 1, 22}
	OIDAES256CBC = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 1, 42}

	OIDPBEWithSHAAnd128BitRC4        = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 12, 1, 1}
	OIDPBEWithSHAAnd40BitRC4         = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 12, 1, 2}
	OIDPBEWithSHAAnd3KeyTripleDESCBC = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 12, 1, 3}
	OIDPBEWithSHAAnd2KeyTripleDESCBC = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 12, 1, 4}
	OIDPBEWithSHAAnd128BitRC2CBC     = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 12, 1, 5}
	OIDPBEWithSHAAnd40BitRC2CBC      = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 12, 1, 6}
)

// names are the names of the object identifiers that errors mention.
var names = []struct {
	oid  asn1.ObjectIdentifier
	name string
}{
	{OIDData, "data"},
	{OIDEncryptedData, "encryptedData"},
	{asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 2}, "signedData"},
	{asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 3}, "envelopedData"},
	{OIDKeyBag, "keyBag"},
	{OIDPKCS8ShroudedKeyBag, "pkcs8ShroudedKeyBag"},
	{OIDCertBag, "certBag"},
	{OIDCRLBag, "crlBag"},
	{OIDSecretBag, "secretBag"},
	{OIDSafeContentsBag, "safeContentsBag"},
	{OIDX509Certificate, "x509Certificate"},
	{asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 22, 2}, "sdsiCertificate"},
	{OIDSHA1, "SHA-1"},
	{OIDSHA256, "SHA-256"},
	{OIDSHA384, "SHA-384"},
	{OIDSHA512, "SHA-512"},
	{OIDHMACWithSHA1, "hmacWithSHA1"},
	{OIDHMACWithSHA256, "hmacWithSHA256"},
	{OIDHMACWithSHA384, "hmacWithSHA384"},
	{OIDHMACWithSHA512, "hmacWithSHA512"},
	{OIDPBKDF2, "PBKDF2"},
	{OIDPBES2, "PBES2"},
	{OIDPBMAC1, "PBMAC1"},
	{OIDAES128CBC, "AES-128-CBC"},
	{OIDAES192CBC, "AES-192-CBC"},
	{OIDAES256CBC, "AES-256-CBC"},
	{OIDPBEWithSHAAnd128BitRC4, "pbeWithSHAAnd128BitRC4"},
	{OIDPBEWithSHAAnd40BitRC4, "pbeWithSHAAnd40BitRC4"},
	{OIDPBEWithSHAAnd3KeyTripleDESCBC, "pbeWithSHAAnd3-KeyTripleDES-CBC"},
	{OIDPBEWithSHAAnd2KeyTripleDESCBC, "pbeWithSHAAnd2-KeyTripleDES-CBC"},
	{OIDPBEWithSHAAnd128BitRC2CBC, "pbeWithSHAAnd128BitRC2-CBC"},
	{OIDPBEWithSHAAnd40BitRC2CBC, "pbeWithSHAAnd40BitRC2-CBC"},
	// PBES1, from RFC 8018.
	{asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 5, 1}, "pbeWithMD2AndDES-CBC (PBES1)"},
	{asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 5, 4}, "pbeWithMD2AndRC2-CBC (PBES1)"},
	{asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 5, 3}, "pbeWithMD5AndDES-CBC (PBES1)"},
	{asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 5, 6}, "pbeWithMD5AndRC2-CBC (PBES1)"},
	{asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 5, 10}, "pbeWithSHA1AndDES-CBC (PBES1)"},
	{asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 5, 11}, "pbeWithSHA1AndRC2-CBC (PBES1)"},
}

// Name returns oid as an error names it: its name and then oid, or oid
// alone if it has no name here.
func Name(oid asn1.ObjectIdentifier) string {
	for _, n := range names {
		if n.oid.Equal(oid) {
			return fmt.Sprintf("%s (%s)", n.name, oid)
		}
	}
	return oid.String()
}

// PFX is RFC 7292's PFX, the outermost structure of a file. A file with no
// MAC has a MacData whose Mac.Algorithm.Algorithm is empty.
type PFX struct {
	Version  int
	AuthSafe ContentInfo
	MacData  MacData `asn1:"optional"`
}

// MacData is RFC 7292's MacData. PBMAC1 ignores MacSalt and Iterations.
type MacData struct {
	Mac        DigestInfo
	MacSalt    []byte
	Iterations int `asn1:"optional,default:1"`
}

// DigestInfo is the DigestInfo of PKCS#7, as MacData holds it.
type DigestInfo struct {
	Algorithm pkix.AlgorithmIdentifier
	Digest    []byte
}

// ContentInfo is the ContentInfo of PKCS#7, restricted to the two content
// types of an AuthenticatedSafe. Content is the [0] EXPLICIT wrapper, and
// its Bytes are the value it holds.
type ContentInfo struct {
	ContentType asn1.ObjectIdentifier
	Content     asn1.RawValue `asn1:"tag:0,explicit"`
}

// EncryptedData is the EncryptedData of PKCS#7.
type EncryptedData struct {
	Version              int
	EncryptedContentInfo EncryptedContentInfo
}

// EncryptedContentInfo is the EncryptedContentInfo of PKCS#7.
type EncryptedContentInfo struct {
	ContentType                asn1.ObjectIdentifier
	ContentEncryptionAlgorithm pkix.AlgorithmIdentifier
	EncryptedContent           []byte `asn1:"tag:0"`
}

// SafeBag is RFC 7292's SafeBag. Value is the [0] EXPLICIT wrapper, and its
// Bytes are the bag's value.
type SafeBag struct {
	ID         asn1.ObjectIdentifier
	Value      asn1.RawValue `asn1:"tag:0,explicit"`
	Attributes []Attribute   `asn1:"set,optional"`
}

// Attribute is a PKCS12Attribute of RFC 7292: an identifier and the SET of
// its values, undecoded.
type Attribute struct {
	ID     asn1.ObjectIdentifier
	Values asn1.RawValue `asn1:"set"`
}

// CertBag is RFC 7292's CertBag. Value is the [0] EXPLICIT wrapper, and its
// Bytes are an OCTET STRING that holds the certificate.
type CertBag struct {
	ID    asn1.ObjectIdentifier
	Value asn1.RawValue `asn1:"tag:0,explicit"`
}

// EncryptedPrivateKeyInfo is the EncryptedPrivateKeyInfo of RFC 5958, the
// value of a pkcs8ShroudedKeyBag.
type EncryptedPrivateKeyInfo struct {
	Algorithm     pkix.AlgorithmIdentifier
	EncryptedData []byte
}

// PBEParams is RFC 7292's pkcs-12PbeParams.
type PBEParams struct {
	Salt       []byte
	Iterations int
}

// PBES2Params is RFC 8018's PBES2-params.
type PBES2Params struct {
	KeyDerivationFunc pkix.AlgorithmIdentifier
	EncryptionScheme  pkix.AlgorithmIdentifier
}

// PBKDF2Params is RFC 8018's PBKDF2-params. A KeyLength of 0 is absent, and
// so is a PRF with an empty Algorithm, which means hmacWithSHA1.
type PBKDF2Params struct {
	Salt           asn1.RawValue
	IterationCount int
	KeyLength      int                      `asn1:"optional"`
	PRF            pkix.AlgorithmIdentifier `asn1:"optional"`
}

// PBMAC1Params is RFC 8018's PBMAC1-params.
type PBMAC1Params struct {
	KeyDerivationFunc pkix.AlgorithmIdentifier
	MessageAuthScheme pkix.AlgorithmIdentifier
}

// unmarshal decodes der, which must hold exactly one DER value, into out.
func unmarshal(der []byte, out any) error {
	rest, err := asn1.Unmarshal(der, out)
	if err != nil {
		return err
	}
	if len(rest) > 0 {
		return fmt.Errorf("%d trailing bytes", len(rest))
	}
	return nil
}
