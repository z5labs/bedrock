// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package pkcs12

import (
	"bytes"
	"crypto/pbkdf2"
	"errors"
	"fmt"
	"hash"
	"unicode/utf8"
)

// MaxIterations is the highest iteration count Decode accepts, for any key
// derivation. It is the cap of Java's PKCS#12 reader.
const MaxIterations = 5_000_000

// checkIterations fails on an iteration count outside 1 to MaxIterations.
// Every derivation is preceded by it, so that a file can't make Decode
// spend minutes deriving a key.
func checkIterations(n int) error {
	if n < 1 || n > MaxIterations {
		return fmt.Errorf("iteration count %d, outside 1 to %d", n, MaxIterations)
	}
	return nil
}

// Password is a password in the two encodings PKCS#12 uses: a BMPString
// for RFC 7292's derivation, and UTF-8 for PBES2 and PBMAC1.
type Password struct {
	bmp, utf8 []byte
}

// passwords returns the forms of password to try, in order. The empty
// password has two: the BMPString of RFC 7292, which is its two-byte zero
// terminator alone, and then zero bytes, as Windows's export wizard writes
// it. Any other password has one, and fails if it isn't valid UTF-8 or
// holds a character outside the Basic Multilingual Plane, which a
// BMPString can't hold.
func passwords(password string) ([]Password, error) {
	if password == "" {
		return []Password{{bmp: []byte{0, 0}}, {}}, nil
	}
	if !utf8.ValidString(password) {
		return nil, errors.New("the password isn't valid UTF-8")
	}
	bmp := make([]byte, 0, 2*len(password)+2)
	for _, r := range password {
		if r > 0xFFFF {
			return nil, errors.New("the password holds a character outside the Basic Multilingual Plane, which a BMPString can't hold")
		}
		bmp = append(bmp, byte(r>>8), byte(r))
	}
	return []Password{{bmp: append(bmp, 0, 0), utf8: []byte(password)}}, nil
}

// The purposes of RFC 7292's derivation, its ID byte.
const (
	purposeKey byte = 1
	purposeIV  byte = 2
	purposeMAC byte = 3
)

// Derive is the key derivation of RFC 7292's Appendix B.2. It returns size
// bytes for purpose id, which is 1 for a key, 2 for an IV and 3 for a MAC
// key, derived with h from password, a BMPString with its terminator, salt
// and iterations. It panics if iterations is less than 1.
func Derive(h func() hash.Hash, password, salt []byte, iterations int, id byte, size int) []byte {
	if iterations < 1 {
		panic(fmt.Sprintf("pkcs12: Derive called with %d iterations", iterations))
	}
	d := h()
	u, v := d.Size(), d.BlockSize()
	diversifier := bytes.Repeat([]byte{id}, v)
	in := append(fill(salt, v), fill(password, v)...)
	out := make([]byte, 0, size+u)
	a := make([]byte, 0, u)
	b := make([]byte, v)
	for {
		d.Reset()
		d.Write(diversifier)
		d.Write(in)
		a = d.Sum(a[:0])
		for range iterations - 1 {
			d.Reset()
			d.Write(a)
			a = d.Sum(a[:0])
		}
		out = append(out, a...)
		if len(out) >= size {
			return out[:size]
		}
		for i := range b {
			b[i] = a[i%u]
		}
		for j := 0; j < len(in); j += v {
			addOne(in[j:j+v], b)
		}
	}
}

// fill returns copies of p, the last one cut short, filling the least
// multiple of v bytes that holds p. It returns nothing for an empty p.
func fill(p []byte, v int) []byte {
	if len(p) == 0 {
		return nil
	}
	out := make([]byte, v*((len(p)+v-1)/v))
	for i := range out {
		out[i] = p[i%len(p)]
	}
	return out
}

// addOne sets x to x+b+1 modulo 2 to the power of its length in bits. x
// and b are big-endian and the same length.
func addOne(x, b []byte) {
	carry := uint(1)
	for i := len(x) - 1; i >= 0; i-- {
		s := uint(x[i]) + uint(b[i]) + carry
		x[i] = byte(s)
		carry = s >> 8
	}
}

// pbkdf2Key is PBKDF2 with its parameters decoded and checked.
type pbkdf2Key struct {
	salt       []byte
	iterations int
	keyLength  int
	prf        func() hash.Hash
}

// pbkdf2Params decodes alg, which must be PBKDF2, and checks its
// parameters: the salt must be an OCTET STRING, the iteration count within
// the limit, and the PRF one of hmacWithSHA1, which is the default,
// hmacWithSHA256, hmacWithSHA384 and hmacWithSHA512.
func pbkdf2Params(alg algorithm) (pbkdf2Key, error) {
	if !alg.Algorithm.Equal(OIDPBKDF2) {
		return pbkdf2Key{}, unsupported("key derivation function", alg.Algorithm)
	}
	var p PBKDF2Params
	if err := unmarshal(alg.Parameters.FullBytes, &p); err != nil {
		return pbkdf2Key{}, fmt.Errorf("PBKDF2 parameters: %w", err)
	}
	if p.Salt.Class != 0 || p.Salt.Tag != 4 || p.Salt.IsCompound {
		return pbkdf2Key{}, errors.New("PBKDF2: a salt that isn't an OCTET STRING")
	}
	if err := checkIterations(p.IterationCount); err != nil {
		return pbkdf2Key{}, fmt.Errorf("PBKDF2: %w", err)
	}
	prf := hmacSHA1
	if len(p.PRF.Algorithm) > 0 {
		var ok bool
		if prf, ok = lookup(hmacs, p.PRF.Algorithm); !ok {
			return pbkdf2Key{}, unsupported("PBKDF2 PRF", p.PRF.Algorithm)
		}
	}
	return pbkdf2Key{salt: p.Salt.Bytes, iterations: p.IterationCount, keyLength: p.KeyLength, prf: prf}, nil
}

// derive returns size bytes of PBKDF2 of password.
func (k pbkdf2Key) derive(password []byte, size int) ([]byte, error) {
	return pbkdf2.Key(k.prf, string(password), k.salt, k.iterations, size)
}
