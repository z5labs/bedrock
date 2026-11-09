// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package pki decodes certificates and keys into the parts of a tls.Config.
//
// Its decoders are strict, and read PEM and PKCS#12: [KeyPair] and
// [PKCS12KeyPair] decode a chain and its key for Certificates, and [Pool]
// and [PKCS12Pool] decode a CertPool for RootCAs or ClientCAs. A
// [Reloader] holds a part that rotates on disk and reads it again while
// the process runs.
//
// pki fixes how a part is decoded and what a reload does. It has no
// tls.Config constructor and no TLS policy. Each framework built on bedrock
// builds its own tls.Config, with its own MinVersion, ClientAuth and
// NextProtos, and decides when to reload. The package pkitest is a
// certificate authority for the tests of code that uses pki.
//
// # Bytes in, decoded values out
//
// pki never opens a file. The caller reads a file through its framework's
// filesystem, so a test reads from its own directory, and each decoder runs
// as a step of a config.Value chain from github.com/z5labs/bedrock/config.
// Here a service reads its key pair from the two files that TLS_CERT_FILE
// and TLS_KEY_FILE name, as a Kubernetes TLS secret mounts them, and
// requires each client to present a certificate signed by a CA in the file
// that TLS_CLIENT_CA_FILE names:
//
//	// FS is what serverTLS reads files through: host's OSFS in production
//	// and hosttest's FS in a test.
//	type FS interface {
//		ReadFile(name string) ([]byte, error)
//	}
//
//	func serverTLS(ctx context.Context, env bedrock.Environment, fsys FS) (*tls.Config, error) {
//		file := func(name string) config.Value[[]byte] {
//			return config.Env(env, name).Parse(fsys.ReadFile)
//		}
//		pair := file("TLS_CERT_FILE").FlatMap(func(certPEM []byte) config.Value[tls.Certificate] {
//			return file("TLS_KEY_FILE").Parse(func(keyPEM []byte) (tls.Certificate, error) {
//				return pki.KeyPair(certPEM, keyPEM)
//			})
//		})
//		return config.Struct(func(r *config.Resolver) *tls.Config {
//			return &tls.Config{
//				Certificates: []tls.Certificate{r.Get(pair)},
//				ClientCAs:    r.Get(file("TLS_CLIENT_CA_FILE").Parse(pki.Pool)),
//				ClientAuth:   tls.RequireAndVerifyClientCert,
//			}
//		}).Read(ctx)
//	}
//
// A PKCS#12 file comes with its password, so its chain reads both, the way
// the pair above reads two files. file is as in serverTLS:
//
//	pair := file("TLS_PKCS12_FILE").FlatMap(func(bundle []byte) config.Value[tls.Certificate] {
//		return config.Env(env, "TLS_PKCS12_PASSWORD").Parse(func(password string) (tls.Certificate, error) {
//			return pki.PKCS12KeyPair(bundle, password)
//		})
//	})
//
// Each error is named after its variable. A variable that isn't set, or a
// file that can't be read, fails under the variable that names it. A pair
// that doesn't decode fails under the variable that completes it:
// TLS_KEY_FILE for the PEM pair, and TLS_PKCS12_PASSWORD for the PKCS#12
// file, where a wrong password fails with an error that wraps
// [ErrIncorrectPassword]. config.Struct reports every failure in one error.
// PEM that arrives inline, from a variable, decodes the same way.
//
// # Strict
//
// Each decoder accepts a subset of what the standard library accepts, or
// go-pkcs12 for PKCS#12, and fails where they would skip a part of the
// input or defer a failure to the first handshake:
//
//   - [Certificates] and [Pool] fail on input that holds no PEM block, and
//     on a block that isn't a CERTIFICATE, has headers or doesn't parse,
//     where x509.CertPool.AppendCertsFromPEM skips it.
//   - [KeyPair] fails on a block that is neither a CERTIFICATE nor a private
//     key, on a certificate of the chain that doesn't parse, and on a second
//     private key, where tls.X509KeyPair skips the block, parses only the
//     leaf, or uses the first key.
//   - Every PEM decoder fails on a "-----BEGIN " that starts no block: a
//     block with no END line, as in a file read while it was being written,
//     or two files joined without a newline. pem.Decode treats either as
//     text and skips it.
//   - [PKCS12KeyPair] and [PKCS12Pool] fail on a bag that their result has
//     no place for, and on a certificate that doesn't parse, where
//     go-pkcs12's DecodeChain and DecodeTrustStore would skip the bag or
//     defer the failure. PKCS12KeyPair also fails on a second private key.
//
// Text between PEM blocks is allowed, because real bundles have it:
// Mozilla's CA bundle has comment headers, and openssl s_client -showcerts
// prints subject= lines. A valid input gives the same result as the
// standard library or go-pkcs12 gives, so switching to pki changes only
// which mistakes fail at startup. Neither key pair decoder verifies the
// chain, its order or its dates. Whether a chain is trusted is the peer's
// question, and failing on a certificate about to expire is a policy, which
// a framework adds as a Validate step in its own chain.
//
// # PKCS#12
//
// PKCS#12 is decoded with the standard library alone, so code that reads a
// .p12 or .pfx file adds no module to its go.mod.
//
//   - It decrypts what OpenSSL, Java, Windows, macOS, NSS and cert-manager
//     write by default: PBES2 with PBKDF2, whose PRF is HMAC with SHA-1,
//     SHA-256, SHA-384 or SHA-512, and AES-CBC; and PBE with SHA-1 and
//     either 3DES or RC2. It verifies an HMAC with SHA-1, SHA-256, SHA-384
//     or SHA-512, keyed by RFC 7292's derivation or by PBMAC1. RC2 is
//     decrypted because cert-manager's default profile, macOS's Keychain,
//     OpenSSL 1.1.1 and Java before 16 write it. RC2-40 protects nothing,
//     but the MAC still catches a wrong password.
//   - It refuses every other algorithm, such as RC4, 2-key 3DES and
//     PKCS#5 v1.5's PBES1, with an error naming it.
//   - An iteration count above 5,000,000, Java's cap, fails rather than
//     stalling startup.
//   - [PKCS12KeyPair] finds the leaf by its key: the leaf is the one
//     certificate whose public key matches the private key, because RFC 7292
//     doesn't order the bags. It fails when no certificate matches, and when
//     more than one does. The rest of the chain follows the leaf in the order
//     of the file.
//   - [PKCS12Pool] reads a trust store as Java means one: every certificate
//     in it is marked trusted with the attribute that Java's keytool writes.
//     A file of certificates without it, such as a key pair's file, fails
//     rather than having its CA certificates trusted.
//   - A file with no MAC decodes only with the empty password, so a password
//     the file doesn't use fails rather than being ignored. The empty
//     password is tried both with and without its terminator, because tools
//     write it both ways.
//   - The file may be BER, which RFC 7292 allows and NSS, Firefox and Bouncy
//     Castle write. It is rewritten to DER before it is decoded, and
//     anything else that DER forbids still fails.
//   - A MAC that doesn't verify fails with an error that wraps
//     [ErrIncorrectPassword], whether the password is wrong or the file
//     changed after it was written.
//
// pki decodes PKCS#12 and doesn't encode it. pkitest writes PKCS#12 for
// tests.
//
// # Rotation
//
// A service whose key pair rotates on disk reads the pair once at startup,
// with the rest of its configuration, so a pair that can't be read fails
// startup. It hands that first value, and the chain that read it, to
// [NewReloader], and serves the pair through [GetCertificate]:
//
//	first, err := pair.Read(ctx)
//	if err != nil {
//		return nil, nil, err
//	}
//	certs := pki.NewReloader(first, pair.Read)
//	return &tls.Config{GetCertificate: pki.GetCertificate(certs)}, certs, nil
//
// Each [Reloader.Reload] reads the chain again. A Reload that fails keeps
// the last good value, which is served as before, and returns load's error
// unchanged, already named after its variable by the chain. Strict decoding
// is what makes that safe: a file read while it is being written has a
// block with no END line, and a certificate and a key from two different
// rotations don't match, so a torn read fails rather than being served,
// and the next Reload after the rotation finishes picks up the new pair.
//
// Leave the config's Certificates empty when GetCertificate is set, because
// a handshake from a client that sends no server name uses Certificates
// instead. A client presents a rotating pair through
// [GetClientCertificate]. A Reload reaches new handshakes only: a
// connection made before it keeps the certificate it was made with.
//
// A Reloader holds a pool too, but putting a pool into a config is building
// a config, which is the framework's. A server returns a Clone of its
// config, with ClientCAs set from the Reloader's Value, from
// GetConfigForClient. A client builds its config for each dial, with
// RootCAs set the same way.
//
// When to call Reload belongs to an application framework, such as
// z5labs/humus, or to the App itself: on a ticker, after a file watcher
// fires, or when an operator asks. pki has no loop. A Reloader starts no
// goroutine, reads no clock and watches no file.
//
// # System roots are an ambient leak
//
// A tls.Config whose RootCAs is nil verifies against the system roots, and
// so does a pool from x509.SystemCertPool. Both read SSL_CERT_FILE and
// SSL_CERT_DIR from the real environment of the process, and the system's
// certificate files from the real filesystem, behind the App's back. Since
// Go 1.27, either variable, when it is set, also replaces the platform
// verifier on macOS and Windows. pki doesn't wrap the system roots, because
// reading them through bedrock's Process would mean reimplementing Go's
// root discovery, and macOS and Windows verify through platform APIs that
// a file can't stand in for.
//
// A test sets RootCAs from its CA, such as pkitest's CA.Pool, so that it
// never depends on the machine it runs on.
//
// The design is discussion #621,
// https://github.com/z5labs/bedrock/discussions/621.
package pki
