// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

//go:build interop

package pki_test

// These tests check pki's decoder and pkitest's encoder against the files
// that OpenSSL and NSS write and read. They need openssl, and NSS's
// certutil and pk12util, on PATH:
//
//	go -C pki test -race -tags interop -run '^TestInterop_' ./...

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	sslmate "software.sslmate.com/src/go-pkcs12"

	"github.com/z5labs/bedrock/pki"
	"github.com/z5labs/bedrock/pki/pkitest"
)

// interopTool runs name with args, with an empty environment so that it
// inherits nothing from the real one, and returns what it writes to its
// standard output. It fails the test, with what the tool wrote to its
// standard error, if the tool fails.
func interopTool(t *testing.T, name string, args ...string) []byte {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), name, args...)
	cmd.Env = []string{}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	require.NoError(t, err, "%s %q: %s", name, args, stderr.String())
	return stdout.Bytes()
}

// interopFile writes data to a new file named name under dir, and returns
// its path.
func interopFile(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, data, 0o600))
	return path
}

// opensslVersion returns the major and minor version that openssl
// version reports, and the report.
func opensslVersion(t *testing.T) (major, minor int, report string) {
	t.Helper()
	report = string(bytes.TrimSpace(interopTool(t, "openssl", "version")))
	m := regexp.MustCompile(`^OpenSSL (\d+)\.(\d+)\.`).FindStringSubmatch(report)
	require.NotNil(t, m, "openssl version: %q", report)
	major, err := strconv.Atoi(m[1])
	require.NoError(t, err)
	minor, err = strconv.Atoi(m[2])
	require.NoError(t, err)
	return major, minor, report
}

func TestInterop_OpenSSL_writes_pki_reads(t *testing.T) {
	major, minor, version := opensslVersion(t)
	t.Logf("%s", version)

	rows := []struct {
		name string
		args []string
		// needs is the oldest OpenSSL, major and minor, that writes the row.
		needs [2]int
	}{
		{name: "defaults"},
		{name: "-descert", args: []string{"-descert"}},
		{name: "-legacy", args: []string{"-legacy"}},
		{name: "-pbmac1_pbkdf2", args: []string{"-pbmac1_pbkdf2"}, needs: [2]int{3, 4}},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			if major < row.needs[0] || major == row.needs[0] && minor < row.needs[1] {
				t.Skipf("this row didn't run: %s is older than OpenSSL %d.%d, which writes %s", version, row.needs[0], row.needs[1], row.name)
			}
			c := pkitest.NewCA(t).Issue(t, "api.internal")
			certPEM, keyPEM := c.PEM()
			dir := t.TempDir()
			out := filepath.Join(dir, "out.p12")
			password := "interop " + row.name
			interopTool(t, "openssl", slices.Concat([]string{
				"pkcs12", "-export",
				"-in", interopFile(t, dir, "chain.pem", certPEM),
				"-inkey", interopFile(t, dir, "key.pem", keyPEM),
				"-out", out,
				"-passout", "pass:" + password,
			}, row.args)...)
			data, err := os.ReadFile(out)
			require.NoError(t, err)

			pair, err := pki.PKCS12KeyPair(data, password)
			require.NoError(t, err)
			requireSamePair(t, c.TLS(), pair, "pki decodes what openssl pkcs12 -export %q writes", row.args)
		})
	}
}

// opensslRead returns the certificates and the private keys that openssl
// pkcs12 -nodes prints for the file data, with password.
func opensslRead(t *testing.T, data []byte, password string) (certs [][]byte, keys [][]byte) {
	t.Helper()
	in := interopFile(t, t.TempDir(), "in.p12", data)
	out := interopTool(t, "openssl", "pkcs12", "-in", in, "-nodes", "-passin", "pass:"+password)
	for {
		var block *pem.Block
		block, out = pem.Decode(out)
		if block == nil {
			return certs, keys
		}
		switch block.Type {
		case "CERTIFICATE":
			certs = append(certs, block.Bytes)
		case "PRIVATE KEY":
			keys = append(keys, block.Bytes)
		default:
			require.Fail(t, fmt.Sprintf("openssl pkcs12 printed a %s block", block.Type))
		}
	}
}

func TestInterop_pkitest_writes_OpenSSL_reads(t *testing.T) {
	ca := pkitest.NewCA(t)
	c := ca.Issue(t, "api.internal")
	tc := c.TLS()

	t.Run("Certificate.PKCS12", func(t *testing.T) {
		bundle, password := c.PKCS12(t)
		certs, keys := opensslRead(t, bundle, password)
		require.Equal(t, tc.Certificate, certs, "openssl prints the leaf, then the CA")
		require.Len(t, keys, 1)
		key, err := x509.ParsePKCS8PrivateKey(keys[0])
		require.NoError(t, err)
		require.True(t, tc.PrivateKey.(*ecdsa.PrivateKey).Equal(key), "openssl prints the certificate's key")
	})

	t.Run("CA.PKCS12", func(t *testing.T) {
		bundle, password := ca.PKCS12(t)
		certs, keys := opensslRead(t, bundle, password)
		require.Equal(t, [][]byte{tc.Certificate[1]}, certs, "openssl prints the CA")
		require.Empty(t, keys)
	})
}

func TestInterop_NSS_writes_pki_reads(t *testing.T) {
	c := pkitest.NewCA(t).Issue(t, "api.internal")
	bundle, password := c.PKCS12(t)
	dir := t.TempDir()
	db := "sql:" + filepath.Join(dir, "nssdb")
	require.NoError(t, os.Mkdir(filepath.Join(dir, "nssdb"), 0o700))
	out := filepath.Join(dir, "out.p12")

	interopTool(t, "certutil", "-N", "-d", db, "--empty-password")
	interopTool(t, "pk12util", "-i", interopFile(t, dir, "in.p12", bundle), "-d", db, "-W", password, "-K", "")
	// pkitest's file names no nickname, so pk12util names the certificate
	// by its common name.
	interopTool(t, "pk12util", "-o", out, "-n", "api.internal", "-d", db, "-W", password, "-K", "")
	data, err := os.ReadFile(out)
	require.NoError(t, err)

	require.GreaterOrEqual(t, len(data), 2, "pk12util -o writes a file")
	require.Equal(t, []byte{0x30, 0x80}, data[:2], "pk12util writes BER, with an indefinite length")
	pair, err := pki.PKCS12KeyPair(data, password)
	require.NoError(t, err)
	requireSamePair(t, c.TLS(), pair, "pki decodes what pk12util -o writes")

	_, _, _, err = sslmate.DecodeChain(data, password)
	require.Error(t, err, "go-pkcs12 rejects the BER that pk12util writes")
}
