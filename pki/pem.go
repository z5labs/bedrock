// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package pki

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
)

// begin opens every PEM block. pem.Decode treats one that starts no block
// it can return as text, so blocks fails on it instead.
var begin = []byte("-----BEGIN ")

// errNoBlock is the error for a "-----BEGIN " that starts no block
// pem.Decode returns: a block with no END line, or two files joined without
// a newline, which hides both of the blocks they meet at.
var errNoBlock = errors.New("-----BEGIN starts no PEM block: is an END line missing, or two files joined without a newline?")

// blocks calls f with each PEM block in data, in order, and its position,
// counting from 1. It stops at the first error f returns. Text between
// blocks is ignored, unless it holds a "-----BEGIN ", which fails as the
// position of the block it should have started. blocks reports how many
// blocks it decoded.
func blocks(data []byte, f func(n int, b *pem.Block) error) (int, error) {
	rest := data
	for n := 1; ; n++ {
		b, after := pem.Decode(rest)
		if b == nil {
			if bytes.Contains(rest, begin) {
				return n - 1, fmt.Errorf("block %d: %w", n, errNoBlock)
			}
			return n - 1, nil
		}
		// What pem.Decode consumed is the text it skipped and then the
		// block, whose own BEGIN line is the only one it may hold. A
		// second is a BEGIN in the text, which starts no block.
		if bytes.Count(rest[:len(rest)-len(after)], begin) > 1 {
			return n - 1, fmt.Errorf("block %d: %w", n, errNoBlock)
		}
		if err := f(n, b); err != nil {
			return n, err
		}
		rest = after
	}
}

// Certificates decodes the certificates in pemData, in order. pemData is a
// series of PEM blocks. Text between blocks is ignored, as in a CA bundle
// with comments. Certificates fails on data that holds no block, on a block
// that isn't a CERTIFICATE, has headers or doesn't parse, and on a block
// with no END line. Its errors name a block by its position, counting
// from 1, and wrap x509's parse error.
func Certificates(pemData []byte) ([]*x509.Certificate, error) {
	var certs []*x509.Certificate
	n, err := blocks(pemData, func(n int, b *pem.Block) error {
		if b.Type != "CERTIFICATE" {
			return fmt.Errorf("block %d: a %q block, not a CERTIFICATE", n, b.Type)
		}
		if len(b.Headers) > 0 {
			return fmt.Errorf("block %d: a CERTIFICATE block with headers", n)
		}
		cert, err := x509.ParseCertificate(b.Bytes)
		if err != nil {
			return fmt.Errorf("block %d: %w", n, err)
		}
		certs = append(certs, cert)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("pki: %w", err)
	}
	if n == 0 {
		return nil, errors.New("pki: no PEM block")
	}
	return certs, nil
}

// Pool returns a CertPool that holds the certificates in pemData, decoded
// as by Certificates, for a tls.Config's RootCAs or ClientCAs. Where
// x509.CertPool.AppendCertsFromPEM skips a block it can't use, Pool fails.
func Pool(pemData []byte) (*x509.CertPool, error) {
	certs, err := Certificates(pemData)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	for _, cert := range certs {
		pool.AddCert(cert)
	}
	return pool, nil
}

// KeyPair decodes certPEM's certificate chain, leaf first, and keyPEM's
// private key into a tls.Certificate for a tls.Config's Certificates, with
// Leaf set. Like tls.X509KeyPair, it takes a file that holds both as both
// arguments, so certPEM may hold the key and keyPEM may hold the chain.
//
// It accepts what X509KeyPair accepts and fails where X509KeyPair would
// skip or defer: on a block in either input that is neither a CERTIFICATE
// nor a private key, on a certificate of the chain that doesn't parse, on
// a second private key in keyPEM, and on a block with no END line. A key
// that doesn't match the leaf fails, as in X509KeyPair, and so does a key
// encrypted with a password.
func KeyPair(certPEM, keyPEM []byte) (tls.Certificate, error) {
	_, err := blocks(certPEM, func(n int, b *pem.Block) error {
		isKey, err := keyPairBlock(n, b)
		if err != nil || isKey {
			return err
		}
		// X509KeyPair parses only the leaf, so a certificate of the chain
		// that doesn't parse would reach the handshake.
		if _, err := x509.ParseCertificate(b.Bytes); err != nil {
			return fmt.Errorf("block %d: %w", n, err)
		}
		return nil
	})
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("pki: certPEM: %w", err)
	}

	keys := 0
	_, err = blocks(keyPEM, func(n int, b *pem.Block) error {
		isKey, err := keyPairBlock(n, b)
		if err != nil || !isKey {
			return err
		}
		keys++
		if keys > 1 {
			return fmt.Errorf("block %d: a second private key", n)
		}
		return nil
	})
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("pki: keyPEM: %w", err)
	}

	// Both inputs have passed, so X509KeyPair reads the whole of each, and
	// what it returns, Leaf included, is KeyPair's result.
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("pki: %w", err)
	}
	return pair, nil
}

// keyPairBlock reports whether b, the nth block of an input to KeyPair, is
// a private key, as tls.X509KeyPair decides, and fails if it is neither a
// private key nor a CERTIFICATE.
func keyPairBlock(n int, b *pem.Block) (bool, error) {
	switch {
	case b.Type == "PRIVATE KEY" || strings.HasSuffix(b.Type, " PRIVATE KEY"):
		return true, nil
	case b.Type == "CERTIFICATE":
		return false, nil
	default:
		return false, fmt.Errorf("block %d: a %q block, neither a CERTIFICATE nor a private key", n, b.Type)
	}
}
