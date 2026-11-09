// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package pkcs12

import (
	"bytes"
	"encoding/asn1"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// h decodes s, hex with spaces.
func h(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(strings.ReplaceAll(s, " ", ""))
	require.NoError(t, err)
	return b
}

// nested returns n SEQUENCEs of indefinite length, each inside the last,
// around an INTEGER.
func nested(n int) []byte {
	return append(append(bytes.Repeat([]byte{0x30, 0x80}, n), 2, 1, 5), bytes.Repeat([]byte{0, 0}, n)...)
}

func TestToDER_rewrites_BER(t *testing.T) {
	long := bytes.Repeat([]byte{0xaa}, 200)
	longer := bytes.Repeat([]byte{0xbb}, 300)
	for _, row := range []struct {
		name string
		ber  []byte
		want []byte
	}{
		{"DER, unchanged", h(t, "30 06 02 01 05 04 01 aa"), h(t, "30 06 02 01 05 04 01 aa")},
		{"an indefinite length", h(t, "30 80 02 01 05 00 00"), h(t, "30 03 02 01 05")},
		{"an indefinite length inside a definite one", h(t, "30 07 30 80 02 01 05 00 00"), h(t, "30 05 30 03 02 01 05")},
		{"a constructed OCTET STRING", h(t, "24 80 04 02 aa bb 04 01 cc 00 00"), h(t, "04 03 aa bb cc")},
		{"a constructed OCTET STRING of a definite length", h(t, "24 06 04 01 aa 04 01 bb"), h(t, "04 02 aa bb")},
		{"a constructed OCTET STRING inside another", h(t, "24 80 24 80 04 01 aa 00 00 04 01 bb 00 00"), h(t, "04 02 aa bb")},
		{"an empty constructed OCTET STRING", h(t, "24 80 00 00"), h(t, "04 00")},
		{"a segment of a long length", append(append(h(t, "24 80 04 81 c8"), long...), 0, 0), append(h(t, "04 81 c8"), long...)},
		{"contents of a length in two octets", append(append(h(t, "30 80 04 82 01 2c"), longer...), 0, 0), append(h(t, "30 82 01 30 04 82 01 2c"), longer...)},
		{"an implicitly tagged constructed value, whose segments stay", h(t, "a0 80 04 01 aa 04 01 bb 00 00"), h(t, "a0 06 04 01 aa 04 01 bb")},
		{"a high tag number", h(t, "3f 81 00 80 1f 81 00 01 05 00 00"), h(t, "3f 81 00 05 1f 81 00 01 05")},
		{"nesting of 64", nested(64), derNested(t, 64)},
	} {
		t.Run(row.name, func(t *testing.T) {
			got, err := toDER(row.ber)
			require.NoError(t, err)
			require.Equal(t, row.want, got)
		})
	}
}

// derNested is nested(n) in DER.
func derNested(t *testing.T, n int) []byte {
	t.Helper()
	der := []byte{2, 1, 5}
	for range n {
		der = mustMarshal(t, asn1.RawValue{Tag: asn1.TagSequence, IsCompound: true, Bytes: der})
	}
	return der
}

func TestToDER_fails(t *testing.T) {
	for _, row := range []struct {
		name    string
		ber     []byte
		wantErr string
	}{
		{"nothing", nil, "BER at offset 0: a value past the end of the input"},
		{"a tag past the end", h(t, "1f 81"), "BER at offset 0: a tag past the end of the input"},
		{"no length", h(t, "30"), "BER at offset 1: a length past the end of the input"},
		{"an indefinite length on a primitive value", h(t, "04 80 00 00"), "BER at offset 1: an indefinite length on a primitive value"},
		{"a length in more than 4 octets", h(t, "04 85 00 00 00 00 01 aa"), "BER at offset 1: a length in more than 4 octets"},
		{"length octets past the end", h(t, "04 82 01"), "BER at offset 1: a length past the end of the input"},
		{"a length in more octets than it needs", h(t, "04 81 01 aa"), "BER at offset 1: a length in more octets than it needs"},
		{"a length with a leading zero", append(h(t, "04 82 00 80"), make([]byte, 128)...), "BER at offset 1: a length in more octets than it needs"},
		{"a length in more octets than it needs, inside an indefinite length", h(t, "30 80 04 81 01 aa 00 00"), "BER at offset 3: a length in more octets than it needs"},
		{"a length past the end", h(t, "04 05 aa"), "BER at offset 1: a length past the end of the input"},
		{"a length past the end of its parent", h(t, "30 03 04 05 aa aa aa aa aa"), "BER at offset 3: a length past the end of the input"},
		{"a missing end-of-contents", h(t, "30 80 02 01 05"), "BER at offset 0: a missing end-of-contents"},
		{"a missing end-of-contents, inside a definite length", h(t, "30 05 30 80 02 01 05"), "BER at offset 2: a missing end-of-contents"},
		{"an end-of-contents inside a definite length", h(t, "30 02 00 00"), "BER at offset 2: an end-of-contents inside a definite length"},
		{"an end-of-contents with a length", h(t, "30 80 00 01 00 00 00"), "BER at offset 2: an end-of-contents with a length"},
		{"an end-of-contents outside an indefinite length", h(t, "00 00"), "BER at offset 0: an end-of-contents outside an indefinite length"},
		{"a segment that isn't an OCTET STRING", h(t, "24 80 02 01 05 00 00"), "BER at offset 2: a segment of a constructed OCTET STRING that isn't an OCTET STRING"},
		{"nesting deeper than 64", nested(65), "BER at offset 128: nesting deeper than 64"},
		{"trailing bytes", h(t, "02 01 05 00"), "1 trailing bytes"},
	} {
		t.Run(row.name, func(t *testing.T) {
			got, err := toDER(row.ber)
			require.EqualError(t, err, row.wantErr)
			require.Nil(t, got)
		})
	}
}
