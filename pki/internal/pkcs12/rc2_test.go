// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package pkcs12

import (
	"encoding/hex"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRC2_decrypts_RFC_2268s_test_vectors(t *testing.T) {
	// RFC 2268's section 5, with plaintext and ciphertext swapped.
	for _, row := range []struct {
		key        string
		effective  int
		plaintext  string
		ciphertext string
	}{
		{"0000000000000000", 63, "0000000000000000", "ebb773f993278eff"},
		{"ffffffffffffffff", 64, "ffffffffffffffff", "278b27e42e2f0d49"},
		{"3000000000000000", 64, "1000000000000001", "30649edf9be7d2c2"},
		{"88", 64, "0000000000000000", "61a8a244adacccf0"},
		{"88bca90e90875a", 64, "0000000000000000", "6ccf4308974c267f"},
		{"88bca90e90875a7f0f79c384627bafb2", 64, "0000000000000000", "1a807d272bbe5db1"},
		{"88bca90e90875a7f0f79c384627bafb2", 128, "0000000000000000", "2269552ab0f85ca6"},
		{"88bca90e90875a7f0f79c384627bafb216f80a6f85920584c42fceb0be255daf1e", 129, "0000000000000000", "5b78d3a43dfff1f1"},
	} {
		t.Run(row.key+"/"+strconv.Itoa(row.effective), func(t *testing.T) {
			key, err := hex.DecodeString(row.key)
			require.NoError(t, err)
			ciphertext, err := hex.DecodeString(row.ciphertext)
			require.NoError(t, err)

			block, err := rc2Cipher(row.effective)(key)
			require.NoError(t, err)
			require.Equal(t, 8, block.BlockSize())
			got := make([]byte, 8)
			block.Decrypt(got, ciphertext)
			require.Equal(t, row.plaintext, hex.EncodeToString(got))
		})
	}
}

func TestRC2_Encrypt_panics(t *testing.T) {
	block := newRC2(make([]byte, 16), 128)
	require.PanicsWithValue(t, "pki: RC2 is decrypt-only", func() {
		block.Encrypt(make([]byte, 8), make([]byte, 8))
	})
}
