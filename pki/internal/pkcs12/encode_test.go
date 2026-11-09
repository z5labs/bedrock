// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package pkcs12

import (
	"crypto/rand"
	"crypto/sha1"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

// failingReader returns n bytes of rand and then fails.
type failingReader struct {
	n int
}

var errRead = errors.New("the reader failed")

func (r *failingReader) Read(p []byte) (int, error) {
	if r.n == 0 {
		return 0, errRead
	}
	p = p[:min(len(p), r.n)]
	r.n -= len(p)
	return rand.Read(p)
}

func TestEncode_decodes_to_the_key_and_the_chain(t *testing.T) {
	leaf, ca := newTestCert(t), newTestCert(t)
	key := leaf.pkcs8(t)

	data, err := Encode(rand.Reader, key, [][]byte{leaf.der, ca.der}, testPassword)
	require.NoError(t, err)

	bags, pw, err := Decode(data, testPassword)
	require.NoError(t, err)
	require.Len(t, bags, 3)

	fingerprint := sha1.Sum(leaf.der)
	localKeyID := attribute(OIDLocalKeyID, marshal(fingerprint[:]))

	for i, want := range [][]byte{leaf.der, ca.der} {
		require.True(t, bags[i].ID.Equal(OIDCertBag), "bag %d is a certBag", i+1)
		der, err := bags[i].Certificate()
		require.NoError(t, err)
		require.Equal(t, want, der, "the certificates are leaf first")
	}
	require.Len(t, bags[0].Attributes, 1)
	require.True(t, bags[0].Attributes[0].ID.Equal(OIDLocalKeyID))
	require.Equal(t, localKeyID.Values.Bytes, bags[0].Attributes[0].Values.Bytes, "the leaf's localKeyId is its SHA-1")
	require.Empty(t, bags[1].Attributes, "the CA's certificate has no attribute")

	require.True(t, bags[2].ID.Equal(OIDPKCS8ShroudedKeyBag))
	require.Len(t, bags[2].Attributes, 1)
	require.True(t, bags[2].Attributes[0].ID.Equal(OIDLocalKeyID))
	require.Equal(t, localKeyID.Values.Bytes, bags[2].Attributes[0].Values.Bytes, "the key's localKeyId is the leaf's SHA-1")
	got, err := bags[2].PrivateKey(pw)
	require.NoError(t, err)
	require.Equal(t, key, got)
	require.False(t, bags[2].Trusted())
}

func TestEncodeTrustStore_decodes_to_the_certificates_marked_trusted(t *testing.T) {
	one, two := newTestCert(t), newTestCert(t)

	data, err := EncodeTrustStore(rand.Reader, [][]byte{one.der, two.der}, testPassword)
	require.NoError(t, err)

	bags, _, err := Decode(data, testPassword)
	require.NoError(t, err)
	require.Len(t, bags, 2)
	for i, want := range [][]byte{one.der, two.der} {
		require.True(t, bags[i].ID.Equal(OIDCertBag))
		require.True(t, bags[i].Trusted(), "bag %d is marked trusted", i+1)
		require.Len(t, bags[i].Attributes, 1)
		require.Equal(t, marshal(OIDAnyExtendedKeyUsage), bags[i].Attributes[0].Values.Bytes, "the trust is for anyExtendedKeyUsage")
		der, err := bags[i].Certificate()
		require.NoError(t, err)
		require.Equal(t, want, der)
	}
}

func TestEncode_fails_on_a_password_a_BMPString_cannot_hold(t *testing.T) {
	c := newTestCert(t)
	for _, password := range []string{"\xff", "emoji 🙂"} {
		_, err := Encode(rand.Reader, c.pkcs8(t), [][]byte{c.der}, password)
		require.Error(t, err, "%q", password)
		_, err = EncodeTrustStore(rand.Reader, [][]byte{c.der}, password)
		require.Error(t, err, "%q", password)
	}
}

func TestEncode_fails_when_the_random_source_fails(t *testing.T) {
	c := newTestCert(t)
	// Each encryption reads its salt and IV, and the MAC its salt.
	const encryption, mac = EncodeSaltSize + 16, EncodeSaltSize

	tests := []struct {
		name  string
		n     int
		trust bool
		want  string
	}{
		{name: "Encode, the certificates", n: 0, want: "the certificates: "},
		{name: "Encode, the key", n: encryption, want: "the key: "},
		{name: "Encode, the MAC", n: 2 * encryption, want: "the MAC: "},
		{name: "EncodeTrustStore, the certificates", n: 0, trust: true, want: "the certificates: "},
		{name: "EncodeTrustStore, the MAC", n: encryption, trust: true, want: "the MAC: "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &failingReader{n: tt.n}
			var err error
			if tt.trust {
				_, err = EncodeTrustStore(r, [][]byte{c.der}, testPassword)
			} else {
				_, err = Encode(r, c.pkcs8(t), [][]byte{c.der}, testPassword)
			}
			require.ErrorIs(t, err, errRead)
			require.ErrorContains(t, err, tt.want)
		})
	}

	t.Run("exactly enough bytes encode", func(t *testing.T) {
		_, err := Encode(&failingReader{n: 2*encryption + mac}, c.pkcs8(t), [][]byte{c.der}, testPassword)
		require.NoError(t, err)
		_, err = EncodeTrustStore(&failingReader{n: encryption + mac}, [][]byte{c.der}, testPassword)
		require.NoError(t, err)
	})
}

func TestEncode_draws_new_salts_and_IVs_each_call(t *testing.T) {
	c := newTestCert(t)
	one, err := Encode(rand.Reader, c.pkcs8(t), [][]byte{c.der}, testPassword)
	require.NoError(t, err)
	two, err := Encode(rand.Reader, c.pkcs8(t), [][]byte{c.der}, testPassword)
	require.NoError(t, err)
	require.NotEqual(t, one, two)
}

func TestMust_panics_on_an_error(t *testing.T) {
	require.Equal(t, 1, must(1, nil))
	require.PanicsWithValue(t, "pkcs12: encoder invariant broken: "+io.EOF.Error(), func() {
		must(0, io.EOF)
	})
}
