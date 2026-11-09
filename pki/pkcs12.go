// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package pki

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"

	"github.com/z5labs/bedrock/pki/internal/pkcs12"
)

// ErrIncorrectPassword is wrapped by the error that PKCS12KeyPair and
// PKCS12Pool return when a file's MAC doesn't verify with the password.
// Either the password is wrong, or the file changed after it was written.
var ErrIncorrectPassword = errors.New("pki: incorrect password")

// PKCS12KeyPair decodes data, a PKCS#12 file such as a .p12 or .pfx that
// holds one private key and its certificate chain, into a tls.Certificate
// for a tls.Config's Certificates, with Leaf set. The leaf is the
// certificate whose public key matches the private key, and the rest of
// the chain follows it in the order of the file.
//
// It verifies the file's MAC with password, and decrypts it with
// password. It decrypts what OpenSSL, Java, Windows, macOS, NSS and
// cert-manager write by default: PBES2 with PBKDF2 and AES-CBC, and PBE
// with SHA-1 and either 3DES or RC2. It fails on any other algorithm,
// naming it, and on an iteration count above 5,000,000. A file with no
// MAC decodes only with the empty password, so a password that the file
// doesn't use fails rather than being ignored. data may be DER or BER.
//
// It fails where go-pkcs12's DecodeChain would skip or defer: on a bag
// that is neither a certificate nor a private key, on a second private
// key, on a certificate that doesn't parse, and on a file in which no
// certificate, or more than one, matches the key. A MAC that doesn't
// verify fails with an error that wraps ErrIncorrectPassword.
func PKCS12KeyPair(data []byte, password string) (tls.Certificate, error) {
	pair, err := pkcs12KeyPair(data, password)
	if err != nil {
		return tls.Certificate{}, pkcs12Error(err)
	}
	return pair, nil
}

// PKCS12Pool decodes data, a PKCS#12 trust store, into a CertPool for a
// tls.Config's RootCAs or ClientCAs. A trust store holds only
// certificates, each marked trusted with the attribute that Java's keytool
// writes. PKCS12Pool verifies and decrypts as PKCS12KeyPair does. It fails
// on a bag that isn't a certificate, such as a private key, and on a
// certificate that isn't marked trusted.
func PKCS12Pool(data []byte, password string) (*x509.CertPool, error) {
	pool, err := pkcs12Pool(data, password)
	if err != nil {
		return nil, pkcs12Error(err)
	}
	return pool, nil
}

func pkcs12KeyPair(data []byte, password string) (tls.Certificate, error) {
	bags, pw, err := pkcs12.Decode(data, password)
	if err != nil {
		return tls.Certificate{}, err
	}
	var key crypto.Signer
	var certs []*x509.Certificate
	for i, bag := range bags {
		n := i + 1
		switch {
		case bag.ID.Equal(pkcs12.OIDKeyBag), bag.ID.Equal(pkcs12.OIDPKCS8ShroudedKeyBag):
			if key != nil {
				return tls.Certificate{}, fmt.Errorf("bag %d: a second private key", n)
			}
			if key, err = privateKey(bag, pw); err != nil {
				return tls.Certificate{}, fmt.Errorf("bag %d: %w", n, err)
			}
		case bag.ID.Equal(pkcs12.OIDCertBag):
			cert, err := certificate(bag)
			if err != nil {
				return tls.Certificate{}, fmt.Errorf("bag %d: %w", n, err)
			}
			certs = append(certs, cert)
		default:
			return tls.Certificate{}, fmt.Errorf("bag %d: a bag of type %s, neither a certificate nor a private key", n, pkcs12.Name(bag.ID))
		}
	}
	if key == nil {
		return tls.Certificate{}, errors.New("no private key")
	}
	if len(certs) == 0 {
		return tls.Certificate{}, errors.New("no certificate")
	}

	// RFC 7292 doesn't order the bags, so the leaf is found by its key.
	pub := key.Public().(interface{ Equal(crypto.PublicKey) bool })
	leaf, matches := 0, 0
	for i, cert := range certs {
		if pub.Equal(cert.PublicKey) {
			leaf = i
			matches++
		}
	}
	switch {
	case matches == 0:
		return tls.Certificate{}, errors.New("no certificate matches the private key")
	case matches > 1:
		return tls.Certificate{}, fmt.Errorf("%d certificates match the private key", matches)
	}
	chain := [][]byte{certs[leaf].Raw}
	for i, cert := range certs {
		if i != leaf {
			chain = append(chain, cert.Raw)
		}
	}
	return tls.Certificate{Certificate: chain, PrivateKey: key, Leaf: certs[leaf]}, nil
}

func pkcs12Pool(data []byte, password string) (*x509.CertPool, error) {
	bags, _, err := pkcs12.Decode(data, password)
	if err != nil {
		return nil, err
	}
	if len(bags) == 0 {
		return nil, errors.New("no certificate")
	}
	pool := x509.NewCertPool()
	for i, bag := range bags {
		n := i + 1
		if !bag.ID.Equal(pkcs12.OIDCertBag) {
			return nil, fmt.Errorf("bag %d: a bag of type %s, not a certificate", n, pkcs12.Name(bag.ID))
		}
		if !bag.Trusted() {
			return nil, fmt.Errorf("bag %d: a certificate that isn't marked trusted with Java's attribute %s", n, pkcs12.OIDJavaTrustStore)
		}
		cert, err := certificate(bag)
		if err != nil {
			return nil, fmt.Errorf("bag %d: %w", n, err)
		}
		pool.AddCert(cert)
	}
	return pool, nil
}

// certificate parses the certificate that bag, a certBag, holds.
func certificate(bag pkcs12.SafeBag) (*x509.Certificate, error) {
	der, err := bag.Certificate()
	if err != nil {
		return nil, fmt.Errorf("certBag: %w", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, fmt.Errorf("certBag: %w", err)
	}
	return cert, nil
}

// privateKey parses the private key that bag, a keyBag or a
// pkcs8ShroudedKeyBag, holds, decrypting it with pw. It fails on a key
// that crypto/tls can't sign with.
func privateKey(bag pkcs12.SafeBag, pw pkcs12.Password) (crypto.Signer, error) {
	name := "pkcs8ShroudedKeyBag"
	if bag.ID.Equal(pkcs12.OIDKeyBag) {
		name = "keyBag"
	}
	der, err := bag.PrivateKey(pw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	key, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	switch key := key.(type) {
	case *rsa.PrivateKey:
		return key, nil
	case *ecdsa.PrivateKey:
		return key, nil
	case ed25519.PrivateKey:
		return key, nil
	default:
		return nil, fmt.Errorf("%s: a %T, which crypto/tls can't sign with", name, key)
	}
}

// pkcs12Error is err, from decoding a PKCS#12 file, as PKCS12KeyPair and
// PKCS12Pool return it.
func pkcs12Error(err error) error {
	if errors.Is(err, pkcs12.ErrMAC) {
		return incorrectPasswordError{err: err}
	}
	return fmt.Errorf("pki: PKCS#12: %w", err)
}

// incorrectPasswordError is the error for a MAC that doesn't verify. It
// wraps ErrIncorrectPassword and the format's error, which names the MAC.
type incorrectPasswordError struct {
	err error
}

func (e incorrectPasswordError) Error() string {
	return "pki: PKCS#12: " + e.err.Error()
}

func (e incorrectPasswordError) Unwrap() []error {
	return []error{ErrIncorrectPassword, e.err}
}
