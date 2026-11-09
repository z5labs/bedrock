// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package pki_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"

	"github.com/z5labs/bedrock/pki"
	"github.com/z5labs/bedrock/pki/pkitest"
)

// A Reloader serves a key pair that rotates on disk. The pair is read once
// at startup, and each Reload reads it again. A handshake made after a
// Reload sees the new leaf. A rotation caught halfway, with the new
// certificate and the old key, fails to reload, and the last good pair is
// served until the rotation finishes.
func ExampleReloader() {
	t := new(exampleTB)
	ctx := context.Background()
	ca := pkitest.NewCA(t)

	// disk stands in for the two files of a Kubernetes TLS secret, which a
	// service reads through its framework's filesystem.
	disk := map[string][]byte{}
	rotate := func(c *pkitest.Certificate) {
		disk["tls.crt"], disk["tls.key"] = c.PEM()
	}
	load := func(context.Context) (tls.Certificate, error) {
		return pki.KeyPair(disk["tls.crt"], disk["tls.key"])
	}

	rotate(ca.Issue(t, "api.internal"))
	first, err := load(ctx)
	if err != nil {
		fmt.Println(err)
		return
	}
	certs := pki.NewReloader(first, load)
	// Certificates is left empty, so that every handshake asks
	// GetCertificate.
	server := &tls.Config{GetCertificate: pki.GetCertificate(certs)}
	fmt.Println("before:", servedNames(ctx, server, ca.Pool()))

	rotate(ca.Issue(t, "api.internal", "v2.api.internal"))
	if err := certs.Reload(ctx); err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("after:", servedNames(ctx, server, ca.Pool()))

	// The certificate has been written and the key has not.
	disk["tls.crt"], _ = ca.Issue(t, "api.internal", "v3.api.internal").PEM()
	fmt.Println("torn:", certs.Reload(ctx))
	fmt.Println("still:", servedNames(ctx, server, ca.Pool()))
	// Output:
	// before: [api.internal]
	// after: [api.internal v2.api.internal]
	// torn: pki: tls: private key does not match public key
	// still: [api.internal v2.api.internal]
}

// servedNames runs a TLS handshake over net.Pipe between a server with
// server and a client of api.internal that trusts roots, and returns the
// DNS names of the leaf the server presented, or the handshake's error.
func servedNames(ctx context.Context, server *tls.Config, roots *x509.CertPool) any {
	srvConn, cliConn := net.Pipe()
	srvErr := make(chan error, 1)
	go func() {
		err := tls.Server(srvConn, server).HandshakeContext(ctx)
		srvErr <- errors.Join(err, srvConn.Close())
	}()
	cli := tls.Client(cliConn, &tls.Config{ServerName: "api.internal", RootCAs: roots})
	err := cli.HandshakeContext(ctx)
	if err := errors.Join(err, cliConn.Close(), <-srvErr); err != nil {
		return err
	}
	return cli.ConnectionState().PeerCertificates[0].DNSNames
}
