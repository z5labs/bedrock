// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package pki_test

import (
	"bytes"
	"crypto/x509"
	"errors"
	"fmt"
	"slices"

	"github.com/z5labs/bedrock/pki"
	"github.com/z5labs/bedrock/pki/pkitest"
)

// KeyPair decodes a certificate chain and its key, from the two files a
// Kubernetes TLS secret mounts. A key from another pair fails, and so does
// a chain cut off partway through a block, as in a file read while it was
// being written.
func ExampleKeyPair() {
	t := new(exampleTB)
	ca := pkitest.NewCA(t)
	certPEM, keyPEM := ca.Issue(t, "api.internal", "api.example.com").PEM()

	pair, err := pki.KeyPair(certPEM, keyPEM)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("leaf:", pair.Leaf.DNSNames)
	fmt.Println("chain:", len(pair.Certificate))

	_, otherKey := ca.Issue(t, "api.internal").PEM()
	_, err = pki.KeyPair(certPEM, otherKey)
	fmt.Println(err)

	_, err = pki.KeyPair(certPEM[:len(certPEM)-30], keyPEM)
	fmt.Println(err)
	// Output:
	// leaf: [api.internal api.example.com]
	// chain: 2
	// pki: tls: private key does not match public key
	// pki: certPEM: block 2: -----BEGIN starts no PEM block: is an END line missing, or two files joined without a newline?
}

// Pool decodes a CA bundle for a tls.Config's RootCAs or ClientCAs. Text
// between blocks, such as a comment, is allowed. A block that isn't a
// certificate fails, where x509.CertPool.AppendCertsFromPEM would skip it,
// and so do two files joined without a newline.
func ExamplePool() {
	t := new(exampleTB)
	ca := pkitest.NewCA(t)
	other := pkitest.NewCA(t)
	bundle := slices.Concat([]byte("# the CAs of api.internal\n"), ca.PEM(), other.PEM())

	pool, err := pki.Pool(bundle)
	if err != nil {
		fmt.Println(err)
		return
	}
	leaf := ca.Issue(t, "api.internal").TLS().Leaf
	_, err = leaf.Verify(x509.VerifyOptions{Roots: pool, DNSName: "api.internal"})
	fmt.Println("verified:", err == nil)

	_, keyPEM := ca.Issue(t, "api.internal").PEM()
	_, err = pki.Pool(slices.Concat(ca.PEM(), keyPEM))
	fmt.Println(err)

	joined := slices.Concat(bytes.TrimSuffix(ca.PEM(), []byte("\n")), other.PEM())
	_, err = pki.Pool(joined)
	fmt.Println(err)
	// Output:
	// verified: true
	// pki: block 2: a "PRIVATE KEY" block, not a CERTIFICATE
	// pki: block 1: -----BEGIN starts no PEM block: is an END line missing, or two files joined without a newline?
}

// PKCS12KeyPair decodes a PKCS#12 file, such as a .p12 or .pfx, with its
// password. A wrong password fails with an error that wraps
// ErrIncorrectPassword, and a trust store, which holds no key, fails too.
func ExamplePKCS12KeyPair() {
	t := new(exampleTB)
	ca := pkitest.NewCA(t)
	bundle, password := ca.Issue(t, "api.internal").PKCS12(t)

	pair, err := pki.PKCS12KeyPair(bundle, password)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("leaf:", pair.Leaf.DNSNames)
	fmt.Println("chain:", len(pair.Certificate))

	_, err = pki.PKCS12KeyPair(bundle, "not the password")
	fmt.Println("incorrect password:", errors.Is(err, pki.ErrIncorrectPassword))

	store, password := ca.PKCS12(t)
	_, err = pki.PKCS12KeyPair(store, password)
	fmt.Println(err)
	// Output:
	// leaf: [api.internal]
	// chain: 2
	// incorrect password: true
	// pki: PKCS#12: no private key
}

// PKCS12Pool decodes a PKCS#12 trust store, as Java's keytool writes one,
// for a tls.Config's RootCAs or ClientCAs. A key pair's file isn't a trust
// store: its certificates aren't marked trusted, so it fails rather than
// having its CA certificate trusted.
func ExamplePKCS12Pool() {
	t := new(exampleTB)
	ca := pkitest.NewCA(t)
	store, password := ca.PKCS12(t)

	pool, err := pki.PKCS12Pool(store, password)
	if err != nil {
		fmt.Println(err)
		return
	}
	leaf := ca.Issue(t, "api.internal").TLS().Leaf
	_, err = leaf.Verify(x509.VerifyOptions{Roots: pool, DNSName: "api.internal"})
	fmt.Println("verified:", err == nil)

	bundle, password := ca.Issue(t, "api.internal").PKCS12(t)
	_, err = pki.PKCS12Pool(bundle, password)
	fmt.Println(err)
	// Output:
	// verified: true
	// pki: PKCS#12: bag 1: a certificate that isn't marked trusted with Java's attribute 2.16.840.1.113894.746875.1.1
}
