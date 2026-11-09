// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package pki_test

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	sslmate "software.sslmate.com/src/go-pkcs12"

	"github.com/z5labs/bedrock/pki"
	"github.com/z5labs/bedrock/pki/internal/pkcs12"
)

// This file builds the PKCS#12 tables that the tests and the fuzz test's
// seed corpus share: files that go-pkcs12's encoders write, on which pki
// returns what go-pkcs12 returns, and files that pki rejects, which
// go-pkcs12 accepts or decodes wrongly, or, since v0.7.4, rejects too.

// password is the password of the tables' files, unless a row says
// otherwise.
const password = sslmate.DefaultPassword

// chain is a key and the certificates issued for it, leaf first.
type chain struct {
	name  string
	key   crypto.Signer
	certs []*x509.Certificate
}

// leaf and cas split c into what go-pkcs12's Encode takes.
func (c chain) leaf() *x509.Certificate  { return c.certs[0] }
func (c chain) cas() []*x509.Certificate { return c.certs[1:] }

// certsOf parses the certificates of a PEM chain.
func certsOf(tb testing.TB, pemData []byte) []*x509.Certificate {
	tb.Helper()
	certs, err := pki.Certificates(pemData)
	require.NoError(tb, err)
	return certs
}

// longChain returns a chain of four for key: a leaf, issued by an
// intermediate, issued by another, issued by a root.
func longChain(tb testing.TB, key crypto.Signer) []*x509.Certificate {
	tb.Helper()
	now := time.Now()
	issue := func(name string, pub crypto.PublicKey, parent *x509.Certificate, signer crypto.Signer, isCA bool) *x509.Certificate {
		tmpl := &x509.Certificate{
			Subject:               pkix.Name{CommonName: name},
			NotBefore:             now.Add(-time.Hour),
			NotAfter:              now.Add(24 * time.Hour),
			KeyUsage:              x509.KeyUsageDigitalSignature,
			BasicConstraintsValid: isCA,
			IsCA:                  isCA,
		}
		if isCA {
			tmpl.KeyUsage |= x509.KeyUsageCertSign
		} else {
			tmpl.DNSNames = []string{name}
		}
		if parent == nil {
			parent = tmpl
		}
		der, err := x509.CreateCertificate(rand.Reader, tmpl, parent, pub, signer)
		require.NoError(tb, err)
		cert, err := x509.ParseCertificate(der)
		require.NoError(tb, err)
		return cert
	}
	newKey := func() *ecdsa.PrivateKey {
		k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(tb, err)
		return k
	}
	rootKey, int1Key, int2Key := newKey(), newKey(), newKey()
	root := issue("pki test root "+rand.Text(), &rootKey.PublicKey, nil, rootKey, true)
	int1 := issue("pki test intermediate one", &int1Key.PublicKey, root, rootKey, true)
	int2 := issue("pki test intermediate two", &int2Key.PublicKey, int1, int1Key, true)
	leaf := issue("api.internal", key.Public(), int2, int2Key, false)
	return []*x509.Certificate{leaf, int2, int1, root}
}

// chains are the key pairs the equivalence table encodes.
func chains(tb testing.TB, m *material) []chain {
	tb.Helper()
	return []chain{
		{name: "ECDSA, from pkitest", key: m.ecKey, certs: certsOf(tb, m.certPEM)},
		{name: "RSA", key: m.rsaKey, certs: certsOf(tb, m.rsaChain)},
		{name: "Ed25519", key: m.edKey, certs: certsOf(tb, m.edChain)},
	}
}

// encoders are the encoders of go-pkcs12 that write key pairs pki decodes.
var encoders = []struct {
	name string
	enc  *sslmate.Encoder
}{
	{"LegacyRC2", sslmate.LegacyRC2},
	{"LegacyDES", sslmate.LegacyDES},
	{"Modern2023", sslmate.Modern2023},
	{"Modern2026", sslmate.Modern2026},
}

// p12Row is an input to PKCS12KeyPair or PKCS12Pool.
type p12Row struct {
	name     string
	data     []byte
	password string
}

// encode encodes c with enc and password.
func encode(tb testing.TB, enc *sslmate.Encoder, c chain, password string) []byte {
	tb.Helper()
	data, err := enc.Encode(c.key, c.leaf(), c.cas(), password)
	require.NoError(tb, err)
	return data
}

// validPKCS12Pairs is the equivalence table for PKCS12KeyPair: files on
// which it returns what go-pkcs12's DecodeChain returns.
func validPKCS12Pairs(tb testing.TB, m *material) []p12Row {
	tb.Helper()
	var rows []p12Row
	for _, e := range encoders {
		for _, c := range chains(tb, m) {
			rows = append(rows, p12Row{name: e.name + ", " + c.name, data: encode(tb, e.enc, c, password), password: password})
		}
	}
	long := chain{key: m.ecKey, certs: longChain(tb, m.ecKey)}
	ec := chains(tb, m)[0]
	return append(rows,
		p12Row{name: "Modern2023, a chain of four", data: encode(tb, sslmate.Modern2023, long, password), password: password},
		p12Row{name: "LegacyDES, the empty password", data: encode(tb, sslmate.LegacyDES, ec, ""), password: ""},
		p12Row{name: "Modern2023, the empty password", data: encode(tb, sslmate.Modern2023, ec, ""), password: ""},
		p12Row{name: "Modern2026, the empty password", data: encode(tb, sslmate.Modern2026, ec, ""), password: ""},
		p12Row{name: "LegacyDES, a Latin password", data: encode(tb, sslmate.LegacyDES, ec, "pässwörd"), password: "pässwörd"},
		p12Row{name: "Modern2023, a Latin password", data: encode(tb, sslmate.Modern2023, ec, "pässwörd"), password: "pässwörd"},
		p12Row{name: "LegacyDES, a CJK password", data: encode(tb, sslmate.LegacyDES, ec, "密码"), password: "密码"},
		p12Row{name: "Modern2026, a CJK password", data: encode(tb, sslmate.Modern2026, ec, "密码"), password: "密码"},
	)
}

// validTrustStores is the equivalence table for PKCS12Pool: trust stores
// on which it returns a pool of the certificates go-pkcs12's
// DecodeTrustStore returns.
func validTrustStores(tb testing.TB, m *material) []p12Row {
	tb.Helper()
	cas := certsOf(tb, append(append(append([]byte{}, m.caPEM...), m.ca2PEM...), m.ca3PEM...))
	var rows []p12Row
	for _, e := range append(encoders, struct {
		name string
		enc  *sslmate.Encoder
	}{"Passwordless", sslmate.Passwordless}) {
		pw := password
		if e.enc == sslmate.Passwordless {
			pw = ""
		}
		data, err := e.enc.EncodeTrustStore(cas, pw)
		require.NoError(tb, err)
		rows = append(rows, p12Row{name: e.name, data: data, password: pw})
	}
	return rows
}

// The strictness table builds the files go-pkcs12's encoders can't write
// from the internal package's structures.

// explicit wraps der in a [0] EXPLICIT tag.
func explicit(der []byte) asn1.RawValue {
	return asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true, Bytes: der}
}

// marshal is asn1.Marshal, failing the test on an error.
func marshal(tb testing.TB, v any) []byte {
	tb.Helper()
	der, err := asn1.Marshal(v)
	require.NoError(tb, err)
	return der
}

// safeBag returns a bag of type id whose value is der.
func safeBag(id asn1.ObjectIdentifier, der []byte, attrs ...pkcs12.Attribute) pkcs12.SafeBag {
	return pkcs12.SafeBag{ID: id, Value: explicit(der), Attributes: attrs}
}

// certBag returns a certBag that holds cert.
func certBag(tb testing.TB, cert []byte, attrs ...pkcs12.Attribute) pkcs12.SafeBag {
	tb.Helper()
	value := marshal(tb, pkcs12.CertBag{ID: pkcs12.OIDX509Certificate, Value: explicit(marshal(tb, cert))})
	return safeBag(pkcs12.OIDCertBag, value, attrs...)
}

// keyBag returns a keyBag that holds key, unencrypted.
func keyBag(tb testing.TB, key crypto.PrivateKey) pkcs12.SafeBag {
	tb.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(tb, err)
	return safeBag(pkcs12.OIDKeyBag, der)
}

// trusted is Java's trust attribute, for any extended key usage, as
// go-pkcs12's EncodeTrustStore writes it.
func trusted(tb testing.TB) pkcs12.Attribute {
	tb.Helper()
	anyEKU := marshal(tb, asn1.ObjectIdentifier{2, 5, 29, 37, 0})
	return pkcs12.Attribute{ID: pkcs12.OIDJavaTrustStore, Values: asn1.RawValue{Tag: asn1.TagSet, IsCompound: true, Bytes: anyEKU}}
}

// authSafe returns an AuthenticatedSafe that holds each of contents, a
// SafeContents, as data.
func authSafe(tb testing.TB, contents ...[]pkcs12.SafeBag) []byte {
	tb.Helper()
	cis := []pkcs12.ContentInfo{}
	for _, bags := range contents {
		if bags == nil {
			bags = []pkcs12.SafeBag{}
		}
		cis = append(cis, pkcs12.ContentInfo{ContentType: pkcs12.OIDData, Content: explicit(marshal(tb, marshal(tb, bags)))})
	}
	return marshal(tb, cis)
}

// pfx returns a PFX of authSafe, with mac.
func pfx(tb testing.TB, authSafe []byte, mac pkcs12.MacData) []byte {
	tb.Helper()
	return marshal(tb, pkcs12.PFX{
		Version:  3,
		AuthSafe: pkcs12.ContentInfo{ContentType: pkcs12.OIDData, Content: explicit(marshal(tb, authSafe))},
		MacData:  mac,
	})
}

// noMAC returns a file with no MAC that holds contents.
func noMAC(tb testing.TB, contents ...[]pkcs12.SafeBag) []byte {
	tb.Helper()
	return pfx(tb, authSafe(tb, contents...), pkcs12.MacData{})
}

// bmp encodes password, which is ASCII, as a BMPString with its terminator.
func bmp(password string) []byte {
	var out []byte
	for _, c := range []byte(password) {
		out = append(out, 0, c)
	}
	return append(out, 0, 0)
}

// sha1MAC returns a file that holds contents, with an HMAC-SHA-1 MAC of
// password keyed with RFC 7292's derivation over iterations.
func sha1MAC(tb testing.TB, password string, iterations int, contents ...[]pkcs12.SafeBag) []byte {
	tb.Helper()
	safe := authSafe(tb, contents...)
	salt := []byte(rand.Text()[:8])
	mac := hmac.New(sha1.New, pkcs12.Derive(sha1.New, bmp(password), salt, iterations, 3, sha1.Size))
	mac.Write(safe)
	return pfx(tb, safe, pkcs12.MacData{
		Mac:        pkcs12.DigestInfo{Algorithm: pkix.AlgorithmIdentifier{Algorithm: pkcs12.OIDSHA1, Parameters: asn1.NullRawValue}, Digest: mac.Sum(nil)},
		MacSalt:    salt,
		Iterations: iterations,
	})
}

// pbmac1 returns a file that holds contents, with a PBMAC1 MAC of password
// with HMAC-SHA-256, keyed with PBKDF2-HMAC-SHA-256 for keyLength bytes.
// The parameters name keyLength unless omitKeyLength is set, and salt is
// their salt, whose content the derivation uses whatever its type.
func pbmac1(tb testing.TB, password string, keyLength int, omitKeyLength bool, salt asn1.RawValue, contents ...[]pkcs12.SafeBag) []byte {
	tb.Helper()
	safe := authSafe(tb, contents...)
	key, err := pbkdf2.Key(sha256.New, password, salt.Bytes, 2048, keyLength)
	require.NoError(tb, err)
	mac := hmac.New(sha256.New, key)
	mac.Write(safe)
	params := pkcs12.PBKDF2Params{
		Salt:           salt,
		IterationCount: 2048,
		KeyLength:      keyLength,
		PRF:            pkix.AlgorithmIdentifier{Algorithm: pkcs12.OIDHMACWithSHA256, Parameters: asn1.NullRawValue},
	}
	if omitKeyLength {
		params.KeyLength = 0
	}
	alg := pkix.AlgorithmIdentifier{Algorithm: pkcs12.OIDPBMAC1, Parameters: asn1.RawValue{FullBytes: marshal(tb, pkcs12.PBMAC1Params{
		KeyDerivationFunc: pkix.AlgorithmIdentifier{Algorithm: pkcs12.OIDPBKDF2, Parameters: asn1.RawValue{FullBytes: marshal(tb, params)}},
		MessageAuthScheme: pkix.AlgorithmIdentifier{Algorithm: pkcs12.OIDHMACWithSHA256, Parameters: asn1.NullRawValue},
	})}}
	return pfx(tb, safe, pkcs12.MacData{
		Mac:        pkcs12.DigestInfo{Algorithm: alg, Digest: mac.Sum(nil)},
		MacSalt:    []byte("NOT USED"),
		Iterations: 1,
	})
}

// octets is salt as an OCTET STRING.
func octets(salt []byte) asn1.RawValue {
	return asn1.RawValue{Tag: asn1.TagOctetString, Bytes: salt}
}

// strictRow is an input that pki rejects with wantErr. stdAccepts is
// whether go-pkcs12's DecodeChain, or for a trust store DecodeTrustStore,
// accepts it.
type strictRow struct {
	p12Row
	trustStore bool
	stdAccepts bool
	wantErr    string
}

// strictPKCS12 is the strictness table.
func strictPKCS12(tb testing.TB, m *material) []strictRow {
	tb.Helper()
	ec := chains(tb, m)[0]
	leaf, ca := ec.leaf().Raw, ec.cas()[0].Raw
	other := m.ca.Issue(tb, "other.internal").TLS().Leaf
	cas := certsOf(tb, append(append([]byte{}, m.caPEM...), m.ca2PEM...))
	nested := safeBag(pkcs12.OIDSafeContentsBag, marshal(tb, []pkcs12.SafeBag{certBag(tb, ca)}))
	crl := safeBag(pkcs12.OIDCRLBag, marshal(tb, pkcs12.CertBag{ID: asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 23, 1}, Value: explicit(marshal(tb, []byte("a CRL")))}))
	secret := safeBag(pkcs12.OIDSecretBag, marshal(tb, pkcs12.CertBag{ID: pkcs12.OIDKeyBag, Value: explicit(marshal(tb, []byte("a secret")))}))
	salt := []byte(rand.Text()[:16])
	notOctets := asn1.RawValue{FullBytes: marshal(tb, pkix.AlgorithmIdentifier{Algorithm: asn1.ObjectIdentifier{1, 2, 3}})}
	notOctets.Bytes = notOctets.FullBytes

	row := func(name string, data []byte, pw string, trustStore, stdAccepts bool, wantErr string) strictRow {
		return strictRow{p12Row: p12Row{name: name, data: data, password: pw}, trustStore: trustStore, stdAccepts: stdAccepts, wantErr: wantErr}
	}
	contents := []pkcs12.SafeBag{certBag(tb, leaf), certBag(tb, ca), keyBag(tb, m.ecKey)}
	return []strictRow{
		row("a CRL bag", noMAC(tb, []pkcs12.SafeBag{certBag(tb, leaf), crl, keyBag(tb, m.ecKey)}), "", false, true,
			"pki: PKCS#12: bag 2: a bag of type crlBag (1.2.840.113549.1.12.10.1.4), neither a certificate nor a private key"),
		row("a secret bag", noMAC(tb, []pkcs12.SafeBag{certBag(tb, leaf), keyBag(tb, m.ecKey), secret}), "", false, true,
			"pki: PKCS#12: bag 3: a bag of type secretBag (1.2.840.113549.1.12.10.1.5), neither a certificate nor a private key"),
		row("a nested bag of bags", noMAC(tb, []pkcs12.SafeBag{certBag(tb, leaf), keyBag(tb, m.ecKey)}, []pkcs12.SafeBag{nested}), "", false, true,
			"pki: PKCS#12: bag 3: a bag of type safeContentsBag (1.2.840.113549.1.12.10.1.6), neither a certificate nor a private key"),
		row("a leaf that doesn't match the key", encode(tb, sslmate.Modern2023, chain{key: m.ecKey, certs: []*x509.Certificate{other, ec.cas()[0]}}, password), password, false, true,
			"pki: PKCS#12: no certificate matches the private key"),
		row("two certificates that match the key", encode(tb, sslmate.Modern2023, chain{key: m.ecKey, certs: []*x509.Certificate{ec.leaf(), ec.cas()[0], ec.leaf()}}, password), password, false, true,
			"pki: PKCS#12: 2 certificates match the private key"),
		row("a password given for a file with no MAC", encode(tb, sslmate.Passwordless, ec, ""), password, false, false,
			"pki: PKCS#12: a password for a file with no macData, which decodes only with the empty password"),
		row("an iteration count of 5,000,001", sha1MAC(tb, password, 5_000_001, contents), password, false, true,
			"pki: PKCS#12: macData: iteration count 5000001, outside 1 to 5000000"),
		row("a trust store with an unmarked certificate", noMAC(tb, []pkcs12.SafeBag{certBag(tb, cas[0].Raw, trusted(tb)), certBag(tb, cas[1].Raw)}), "", true, false,
			"pki: PKCS#12: bag 2: a certificate that isn't marked trusted with Java's attribute 2.16.840.1.113894.746875.1.1"),
		row("a trust store with a key", noMAC(tb, []pkcs12.SafeBag{certBag(tb, cas[0].Raw, trusted(tb)), keyBag(tb, m.ecKey)}), "", true, false,
			"pki: PKCS#12: bag 2: a bag of type keyBag (1.2.840.113549.1.12.10.1.1), not a certificate"),
		row("a PBMAC1 key length of 1", pbmac1(tb, password, 1, false, octets(salt), contents), password, false, false,
			"pki: PKCS#12: macData: PBMAC1: PBKDF2 key length 1, outside 20 to 64"),
		row("a PBMAC1 key length of 65", pbmac1(tb, password, 65, false, octets(salt), contents), password, false, false,
			"pki: PKCS#12: macData: PBMAC1: PBKDF2 key length 65, outside 20 to 64"),
		row("a PBMAC1 key length absent", pbmac1(tb, password, 32, true, octets(salt), contents), password, false, false,
			"pki: PKCS#12: macData: PBMAC1: PBKDF2 has no key length"),
		row("a PBMAC1 salt that isn't an OCTET STRING", pbmac1(tb, password, 32, false, notOctets, contents), password, false, false,
			"pki: PKCS#12: macData: PBMAC1: PBKDF2: a salt that isn't an OCTET STRING"),
	}
}

// rfc9579 returns the file of RFC 9579's test vector A.n, from testdata.
func rfc9579(tb testing.TB, n int) []byte {
	tb.Helper()
	b64, err := os.ReadFile(filepath.Join("testdata", "rfc9579-a"+string(rune('0'+n))+".txt"))
	require.NoError(tb, err)
	data, err := base64.StdEncoding.DecodeString(string(b64))
	require.NoError(tb, err)
	return data
}
