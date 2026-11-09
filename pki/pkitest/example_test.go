// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package pkitest_test

import (
	"crypto/x509"
	"errors"
	"fmt"

	"github.com/z5labs/bedrock/pki"
	"github.com/z5labs/bedrock/pki/pkitest"
)

// A CA issues certificates that its Pool trusts. A certificate for another
// name is an Issue for that name, and one the client doesn't trust comes
// from a second NewCA.
func ExampleNewCA() {
	t := new(exampleTB)
	ca := pkitest.NewCA(t)
	opts := x509.VerifyOptions{Roots: ca.Pool(), DNSName: "api.internal"}

	_, err := ca.Issue(t, "api.internal").TLS().Leaf.Verify(opts)
	fmt.Println("trusted:", err == nil)

	_, err = ca.Issue(t, "billing.internal").TLS().Leaf.Verify(opts)
	var wrongName x509.HostnameError
	fmt.Println("wrong name:", errors.As(err, &wrongName))

	_, err = pkitest.NewCA(t).Issue(t, "api.internal").TLS().Leaf.Verify(opts)
	var untrusted x509.UnknownAuthorityError
	fmt.Println("untrusted CA:", errors.As(err, &untrusted))
	// Output:
	// trusted: true
	// wrong name: true
	// untrusted CA: true
}

// PEM is what a test writes where the code under test reads its key pair,
// as the two files of a Kubernetes TLS secret, and pki.KeyPair decodes it.
func ExampleCertificate_PEM() {
	t := new(exampleTB)
	ca := pkitest.NewCA(t)
	certPEM, keyPEM := ca.Issue(t, "api.internal").PEM()

	pair, err := pki.KeyPair(certPEM, keyPEM)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("leaf:", pair.Leaf.DNSNames)
	fmt.Println("chain:", len(pair.Certificate))

	pool, err := pki.Pool(ca.PEM())
	if err != nil {
		fmt.Println(err)
		return
	}
	_, err = pair.Leaf.Verify(x509.VerifyOptions{Roots: pool, DNSName: "api.internal"})
	fmt.Println("trusted by the CA's PEM:", err == nil)
	// Output:
	// leaf: [api.internal]
	// chain: 2
	// trusted by the CA's PEM: true
}

// PKCS12 is a key pair's PKCS#12 file and the new password it is encrypted
// with, which pki.PKCS12KeyPair decodes.
func ExampleCertificate_PKCS12() {
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

	_, again := ca.Issue(t, "api.internal").PKCS12(t)
	fmt.Println("a new password each call:", again != password)
	// Output:
	// leaf: [api.internal]
	// chain: 2
	// a new password each call: true
}
