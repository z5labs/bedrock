// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package pkcs12

import (
	"encoding/asn1"
	"errors"
	"fmt"
	"slices"
)

// Decode decodes data, a PFX, and verifies its MAC with password. It
// returns the bags of every SafeContents, decrypted, in the order of the
// file, and password in the form whose MAC verified, for
// SafeBag.PrivateKey. A file with no MAC decodes only with the empty
// password. A MAC that doesn't verify fails with an error that wraps
// ErrMAC.
func Decode(data []byte, password string) ([]SafeBag, Password, error) {
	var pfx PFX
	if err := unmarshal(data, &pfx); err != nil {
		return nil, Password{}, fmt.Errorf("PFX: %w", err)
	}
	if pfx.Version != 3 {
		return nil, Password{}, fmt.Errorf("PFX: version %d, not 3", pfx.Version)
	}
	if !pfx.AuthSafe.ContentType.Equal(OIDData) {
		return nil, Password{}, fmt.Errorf("authSafe: content type %s, not data", Name(pfx.AuthSafe.ContentType))
	}
	var authSafe []byte
	if err := unmarshal(pfx.AuthSafe.Content.Bytes, &authSafe); err != nil {
		return nil, Password{}, fmt.Errorf("authSafe: %w", err)
	}
	pw, err := verify(pfx.MacData, authSafe, password)
	if err != nil {
		return nil, Password{}, err
	}

	var safe []ContentInfo
	if err := unmarshal(authSafe, &safe); err != nil {
		return nil, Password{}, fmt.Errorf("AuthenticatedSafe: %w", err)
	}
	var bags []SafeBag
	for i, ci := range safe {
		contents, err := safeContents(ci, pw)
		if err != nil {
			return nil, Password{}, fmt.Errorf("AuthenticatedSafe: ContentInfo %d: %w", i+1, err)
		}
		bags = append(bags, contents...)
	}
	return bags, pw, nil
}

// verify verifies m, the MAC of authSafe, with password, and returns the
// form of password whose MAC verified.
func verify(m MacData, authSafe []byte, password string) (Password, error) {
	pws, err := passwords(password)
	if err != nil {
		return Password{}, err
	}
	if len(m.Mac.Algorithm.Algorithm) == 0 {
		if password != "" {
			return Password{}, errors.New("a password for a file with no macData, which decodes only with the empty password")
		}
		return pws[0], nil
	}
	for _, pw := range pws {
		ok, err := m.verify(authSafe, pw)
		if err != nil {
			return Password{}, fmt.Errorf("macData: %w", err)
		}
		if ok {
			return pw, nil
		}
	}
	return Password{}, fmt.Errorf("macData: %w", ErrMAC)
}

// safeContents returns the bags of the SafeContents that ci holds,
// decrypting it with pw if it is encryptedData.
func safeContents(ci ContentInfo, pw Password) ([]SafeBag, error) {
	var data []byte
	switch {
	case ci.ContentType.Equal(OIDData):
		if err := unmarshal(ci.Content.Bytes, &data); err != nil {
			return nil, fmt.Errorf("data: %w", err)
		}
	case ci.ContentType.Equal(OIDEncryptedData):
		var ed EncryptedData
		if err := unmarshal(ci.Content.Bytes, &ed); err != nil {
			return nil, fmt.Errorf("encryptedData: %w", err)
		}
		if ed.Version != 0 {
			return nil, fmt.Errorf("encryptedData: version %d, not 0", ed.Version)
		}
		info := ed.EncryptedContentInfo
		if !info.ContentType.Equal(OIDData) {
			return nil, fmt.Errorf("encryptedData: content type %s, not data", Name(info.ContentType))
		}
		var err error
		if data, err = Decrypt(info.ContentEncryptionAlgorithm, pw, info.EncryptedContent); err != nil {
			return nil, fmt.Errorf("encryptedData: %w", err)
		}
	default:
		return nil, fmt.Errorf("content type %s, neither data nor encryptedData", Name(ci.ContentType))
	}
	var bags []SafeBag
	if err := unmarshal(data, &bags); err != nil {
		return nil, fmt.Errorf("SafeContents: %w", err)
	}
	return bags, nil
}

// Certificate returns the DER of the X.509 certificate that b, a certBag,
// holds.
func (b SafeBag) Certificate() ([]byte, error) {
	var bag CertBag
	if err := unmarshal(b.Value.Bytes, &bag); err != nil {
		return nil, err
	}
	if !bag.ID.Equal(OIDX509Certificate) {
		return nil, fmt.Errorf("a certificate of type %s, not x509Certificate", Name(bag.ID))
	}
	var der []byte
	if err := unmarshal(bag.Value.Bytes, &der); err != nil {
		return nil, err
	}
	return der, nil
}

// PrivateKey returns the DER of the PKCS#8 private key that b holds. A
// keyBag holds it as it is. Any other bag is taken to be a
// pkcs8ShroudedKeyBag, whose key PrivateKey decrypts with pw.
func (b SafeBag) PrivateKey(pw Password) ([]byte, error) {
	der := b.Value.Bytes
	if !b.ID.Equal(OIDKeyBag) {
		var info EncryptedPrivateKeyInfo
		if err := unmarshal(der, &info); err != nil {
			return nil, err
		}
		var err error
		if der, err = Decrypt(info.Algorithm, pw, info.EncryptedData); err != nil {
			return nil, err
		}
	}
	// x509.ParsePKCS8PrivateKey ignores bytes after the key.
	var key asn1.RawValue
	if err := unmarshal(der, &key); err != nil {
		return nil, err
	}
	return key.FullBytes, nil
}

// Trusted reports whether b holds Java's trust attribute, which marks a
// certificate of a trust store as trusted. It is the only attribute the
// format reads.
func (b SafeBag) Trusted() bool {
	return slices.ContainsFunc(b.Attributes, func(a Attribute) bool {
		return a.ID.Equal(OIDJavaTrustStore)
	})
}
