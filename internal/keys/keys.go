// Package keys generates and parses WireGuard (and so AmneziaWG) X25519 keys.
//
// The standard library's crypto/ecdh does the curve arithmetic; nothing here
// depends on the AmneziaWG tools, so keys can be issued on any machine.
package keys

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// Size is the length of a key in bytes.
const Size = 32

// Key is a private or public X25519 key.
type Key [Size]byte

// ErrInvalid is returned for a string that is not a base64-encoded 32-byte key.
var ErrInvalid = errors.New("not a base64-encoded 32-byte key")

// Generate returns a new private key read from r (crypto/rand when nil),
// clamped the way `wg genkey` clamps it.
func Generate(r io.Reader) (Key, error) {
	if r == nil {
		r = rand.Reader
	}
	var k Key
	if _, err := io.ReadFull(r, k[:]); err != nil {
		return Key{}, fmt.Errorf("read randomness: %w", err)
	}
	k[0] &= 248
	k[31] = (k[31] & 127) | 64
	return k, nil
}

// Public derives the public key of a private key.
func (k Key) Public() Key {
	priv, err := ecdh.X25519().NewPrivateKey(k[:])
	if err != nil {
		// NewPrivateKey only fails on a wrong length, which the array type rules out.
		panic(err)
	}
	var pub Key
	copy(pub[:], priv.PublicKey().Bytes())
	return pub
}

// String encodes the key the way WireGuard configuration files carry it.
func (k Key) String() string {
	return base64.StdEncoding.EncodeToString(k[:])
}

// IsZero reports whether the key is all zeroes, i.e. unset.
func (k Key) IsZero() bool {
	return k == Key{}
}

// Parse decodes a base64 key.
func Parse(s string) (Key, error) {
	raw, err := base64.StdEncoding.DecodeString(s)
	if err != nil || len(raw) != Size {
		return Key{}, ErrInvalid
	}
	var k Key
	copy(k[:], raw)
	return k, nil
}
