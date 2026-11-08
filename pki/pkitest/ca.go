// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package pkitest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"net"
	"slices"
	"testing"
	"time"
)

// A certificate is valid from notBefore before the time it is made to
// notAfter after it.
const (
	notBefore = time.Hour
	notAfter  = 24 * time.Hour
)

// CA is a certificate authority generated for one test, with a new ECDSA
// P-256 key. It is valid from an hour before time.Now() to a day after.
// Inside a testing/synctest bubble that is the bubble's clock, so build a
// CA inside the bubble that uses it.
type CA struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
}

// NewCA generates a CA. It fails the test through tb if generation fails.
func NewCA(tb testing.TB) *CA {
	tb.Helper()
	now := time.Now()
	tmpl := &x509.Certificate{
		// A nil SerialNumber makes x509.CreateCertificate generate one that
		// follows RFC 5280.
		Subject:               pkix.Name{CommonName: "pkitest CA " + rand.Text()},
		NotBefore:             now.Add(-notBefore),
		NotAfter:              now.Add(notAfter),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLenZero:        true,
	}
	cert, key := create(tb, "NewCA", tmpl, nil)
	return &CA{cert: cert, key: key}
}

// Pool returns a CertPool holding only the CA's certificate, for a
// tls.Config's RootCAs or ClientCAs.
func (ca *CA) Pool() *x509.CertPool {
	pool := x509.NewCertPool()
	pool.AddCert(ca.cert)
	return pool
}

// PEM returns the CA's certificate as a PEM block, which pki.Pool decodes.
func (ca *CA) PEM() []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.cert.Raw})
}

// Issue returns a certificate signed by the CA, with a new key, for names,
// which are DNS names or IP literals. It is valid for server and client
// authentication, from an hour before time.Now() to a day after. A client
// certificate needs no names.
func (ca *CA) Issue(tb testing.TB, names ...string) *Certificate {
	tb.Helper()
	call := fmt.Sprintf("Issue(%q)", names)
	now := time.Now()
	tmpl := &x509.Certificate{
		NotBefore:   now.Add(-notBefore),
		NotAfter:    now.Add(notAfter),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}
	if len(names) > 0 {
		tmpl.Subject.CommonName = names[0]
	}
	for _, name := range names {
		if ip := net.ParseIP(name); ip != nil {
			tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
			continue
		}
		tmpl.DNSNames = append(tmpl.DNSNames, name)
	}
	leaf, key := create(tb, call, tmpl, ca)
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	check(tb, call, err)
	return &Certificate{leaf: leaf, key: key, pkcs8: pkcs8, ca: ca}
}

// Certificate is a certificate issued by a CA, with its private key.
type Certificate struct {
	leaf  *x509.Certificate
	key   *ecdsa.PrivateKey
	pkcs8 []byte
	ca    *CA
}

// TLS returns the certificate, its chain and its key, for a tls.Config's
// Certificates.
func (c *Certificate) TLS() tls.Certificate {
	return tls.Certificate{
		Certificate: [][]byte{c.leaf.Raw, c.ca.cert.Raw},
		PrivateKey:  c.key,
		Leaf:        c.leaf,
	}
}

// PEM returns the chain, the certificate then the CA's, and the private
// key, each as PEM, which pki.KeyPair decodes: pki.KeyPair(c.PEM()).
func (c *Certificate) PEM() (certPEM, keyPEM []byte) {
	certPEM = slices.Concat(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: c.leaf.Raw}),
		c.ca.PEM(),
	)
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: c.pkcs8})
	return certPEM, keyPEM
}

// create generates an ECDSA P-256 key and a certificate from tmpl for it,
// signed by ca or, when ca is nil, by the key itself. It fails the test
// through tb, naming call, if either step fails.
func create(tb testing.TB, call string, tmpl *x509.Certificate, ca *CA) (*x509.Certificate, *ecdsa.PrivateKey) {
	tb.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	check(tb, call, err)
	parent, signer := tmpl, key
	if ca != nil {
		parent, signer = ca.cert, ca.key
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, parent, &key.PublicKey, signer)
	check(tb, call, err)
	cert, err := x509.ParseCertificate(der)
	check(tb, call, err)
	return cert, key
}

// check fails the test through tb, naming call, if err is not nil. Fatalf
// stops the goroutine, so nothing after a failed check runs.
func check(tb testing.TB, call string, err error) {
	tb.Helper()
	if err != nil {
		tb.Fatalf("pkitest: %s: %v", call, err)
	}
}
