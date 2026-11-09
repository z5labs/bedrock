// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package pkcs12

import (
	"fmt"
)

// maxBERDepth is the most constructed values that toDER nests, one inside
// another, counting the outermost as 1.
const maxBERDepth = 64

// The identifier octets that toDER reads.
const (
	idEndOfContents       = 0x00
	idOctetString         = 0x04
	idConstructedOctetStr = 0x24
	constructedBit        = 0x20
	highTagNumber         = 0x1f
)

// berError is a value that toDER can't rewrite, at offset into its input.
type berError struct {
	offset int
	msg    string
}

func (e berError) Error() string {
	return fmt.Sprintf("BER at offset %d: %s", e.offset, e.msg)
}

// toDER rewrites b, which must hold exactly one BER value, to DER. RFC 7292
// allows BER, and encoding/asn1 decodes DER only.
//
// It rewrites two things and nothing else: an indefinite length becomes the
// definite length of the rewritten contents, and a constructed OCTET STRING
// becomes the primitive one of its segments, joined. It copies the
// identifier octets of every value and the contents of every primitive one
// as they are, so the content octets of an OCTET STRING, such as a
// certificate, are never rewritten, and encoding/asn1 still rejects
// anything else that DER forbids. It fails, naming the offset, on a length
// in more octets than it needs, a length past the end of b, an
// end-of-contents that is missing or misplaced, and nesting deeper than
// 64.
func toDER(b []byte) ([]byte, error) {
	r := berReader{b: b}
	out, eoc, err := r.value(len(b), 1)
	if err != nil {
		return nil, err
	}
	if eoc {
		return nil, berError{0, "an end-of-contents outside an indefinite length"}
	}
	if r.off < len(b) {
		return nil, fmt.Errorf("%d trailing bytes", len(b)-r.off)
	}
	return out, nil
}

// berReader reads BER values from b, from off.
type berReader struct {
	b   []byte
	off int
}

// value reads the value at r.off, which must end by end, at depth, and
// returns it as DER. eoc reports that it was an end-of-contents instead,
// whose DER is nothing.
func (r *berReader) value(end, depth int) (der []byte, eoc bool, err error) {
	start := r.off
	id, err := r.identifier(end)
	if err != nil {
		return nil, false, err
	}
	constructed := id[0]&constructedBit != 0
	n, indefinite, err := r.length(end, constructed)
	if err != nil {
		return nil, false, err
	}
	if id[0] == idEndOfContents {
		if indefinite || n != 0 {
			return nil, false, berError{start, "an end-of-contents with a length"}
		}
		return nil, true, nil
	}
	if !constructed {
		r.off += n
		return r.b[start:r.off], false, nil
	}

	if depth > maxBERDepth {
		return nil, false, berError{start, fmt.Sprintf("nesting deeper than %d", maxBERDepth)}
	}
	contentsEnd := end
	if !indefinite {
		contentsEnd = r.off + n
	}
	var contents []byte
	segments := id[0] == idConstructedOctetStr
	for {
		if r.off == contentsEnd {
			if indefinite {
				return nil, false, berError{start, "a missing end-of-contents"}
			}
			break
		}
		childStart := r.off
		child, eoc, err := r.value(contentsEnd, depth+1)
		if err != nil {
			return nil, false, err
		}
		if eoc {
			if !indefinite {
				return nil, false, berError{childStart, "an end-of-contents inside a definite length"}
			}
			break
		}
		if segments {
			if child[0] != idOctetString {
				return nil, false, berError{childStart, "a segment of a constructed OCTET STRING that isn't an OCTET STRING"}
			}
			child = child[headerLength(child):]
		}
		contents = append(contents, child...)
	}
	if segments {
		id = []byte{idOctetString}
	}
	out := append(append([]byte{}, id...), derLength(len(contents))...)
	return append(out, contents...), false, nil
}

// identifier reads the identifier octets at r.off, which must end by end.
func (r *berReader) identifier(end int) ([]byte, error) {
	start := r.off
	if start >= end {
		return nil, berError{start, "a value past the end of the input"}
	}
	r.off++
	if r.b[start]&highTagNumber == highTagNumber {
		for {
			if r.off >= end {
				return nil, berError{start, "a tag past the end of the input"}
			}
			r.off++
			if r.b[r.off-1]&0x80 == 0 {
				break
			}
		}
	}
	return r.b[start:r.off], nil
}

// length reads the length octets at r.off, which with the contents they
// count must end by end. It reports an indefinite length, which only a
// constructed value may have, with indefinite.
func (r *berReader) length(end int, constructed bool) (n int, indefinite bool, err error) {
	start := r.off
	if start >= end {
		return 0, false, berError{start, "a length past the end of the input"}
	}
	first := r.b[start]
	r.off++
	switch {
	case first == 0x80:
		if !constructed {
			return 0, false, berError{start, "an indefinite length on a primitive value"}
		}
		return 0, true, nil
	case first < 0x80:
		n = int(first)
	default:
		size := int(first & 0x7f)
		if size > 4 {
			return 0, false, berError{start, "a length in more than 4 octets"}
		}
		if r.off+size > end {
			return 0, false, berError{start, "a length past the end of the input"}
		}
		for _, c := range r.b[r.off : r.off+size] {
			n = n<<8 | int(c)
		}
		r.off += size
		if r.b[start+1] == 0 || n < 0x80 {
			return 0, false, berError{start, "a length in more octets than it needs"}
		}
	}
	if n > end-r.off {
		return 0, false, berError{start, "a length past the end of the input"}
	}
	return n, false, nil
}

// derLength returns the DER length octets of n.
func derLength(n int) []byte {
	if n < 0x80 {
		return []byte{byte(n)}
	}
	var octets []byte
	for ; n > 0; n >>= 8 {
		octets = append([]byte{byte(n)}, octets...)
	}
	return append([]byte{0x80 | byte(len(octets))}, octets...)
}

// headerLength returns the length of the header of der, a DER value with a
// one-octet identifier.
func headerLength(der []byte) int {
	if der[1] < 0x80 {
		return 2
	}
	return 2 + int(der[1]&0x7f)
}
