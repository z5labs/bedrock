// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package pkitest is a certificate authority generated for one test.
//
// A [CA] is a self-signed certificate and its key, made by [NewCA] with a
// new ECDSA P-256 key, and [CA.Issue] signs a [Certificate] for the names a
// test serves or dials. Every key, certificate and password is generated
// when the test runs, and none is checked into the repository. A fixed key
// is a secret anyone can read, a fixed certificate expires and fails the
// build long after anyone remembers making it, and a fixed password is one
// more secret in the repository. A CA of its own also keeps each test from
// trusting another test's certificates.
//
// # Inside a synctest bubble
//
// A CA and every certificate it issues are valid from an hour before
// time.Now() to a day after. Inside a testing/synctest bubble, time.Now is
// the bubble's clock, so build the CA inside the bubble that uses it, with
// the bubble's t:
//
//	func TestServe_presents_its_certificate(t *testing.T) {
//		synctest.Test(t, func(t *testing.T) {
//			ca := pkitest.NewCA(t)
//			certPEM, keyPEM := ca.Issue(t, "api.internal").PEM()
//			// ... write certPEM and keyPEM where the code under test reads
//			// them, and dial it with &tls.Config{RootCAs: ca.Pool(),
//			// ServerName: "api.internal"} ...
//		})
//	}
//
// The validity is fixed, and there are no options. A test of an expired
// certificate sleeps past it inside the bubble, which takes no real time:
// after time.Sleep(25 * time.Hour), a client's handshake fails with an
// error that wraps an x509.CertificateInvalidError whose Reason is
// x509.Expired.
//
// # Outputs
//
// A CA and a Certificate each come in the forms that a tls.Config, or the
// code under test, takes:
//
//   - [Certificate.TLS] is a tls.Certificate, with its chain and key, for a
//     tls.Config's Certificates, and [CA.Pool] is a CertPool holding only
//     the CA, for RootCAs or ClientCAs. A test passes these straight to its
//     own side of a handshake.
//   - [Certificate.PEM] returns the chain and the key apart, for the
//     two-file layout of a Kubernetes TLS secret, and [CA.PEM] returns the
//     CA's certificate. pki.KeyPair and pki.Pool decode them. A test that
//     wants one file joins the two with slices.Concat, which is safe
//     because every block ends with a newline.
//   - [Certificate.PKCS12] returns a PKCS#12 file of the key and the chain,
//     encrypted with the algorithms OpenSSL 3 uses by default, and
//     [CA.PKCS12] returns a trust store of the CA's certificate, marked
//     trusted as Java's keytool marks it. Each comes with a new password.
//     pki.PKCS12KeyPair and pki.PKCS12Pool decode them.
//
// # Failure cases
//
// A test of a failure builds it from the same pieces. A certificate for the
// wrong name is an Issue for another name, and a CA the peer doesn't trust
// is a second NewCA, whose certificates the first CA's pool rejects.
//
// The design is discussion #621,
// https://github.com/z5labs/bedrock/discussions/621.
package pkitest
