// Package secrets seals a value the product has to keep, so a credential in the database is
// unreadable to anyone holding the database alone.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// KeyLength is the key width AES-256 takes, in bytes.
const KeyLength = 32

// envelopePrefix tags the key generation a value was sealed under, so a later rotation can tell
// one from the next instead of guessing.
const envelopePrefix = "v1."

// ErrNoKey is returned by a sealer built without a key: sealing is refused rather than skipped,
// so a deployment that never set one cannot store a credential in the clear.
var ErrNoKey = errors.New("no encryption key configured")

// ErrMalformed is returned when a value is not an envelope this package sealed, tampering
// included — which of the two it was is not something a caller can act on.
var ErrMalformed = errors.New("value is not a sealed envelope")

// AESGCM seals values with AES-256-GCM under one key.
type AESGCM struct {
	aead cipher.AEAD
}

// NewAESGCM builds a sealer over a KeyLength-byte key. An empty key yields one that refuses every
// call, which is what lets a checkout with no key still boot.
func NewAESGCM(key []byte) (*AESGCM, error) {
	if len(key) == 0 {
		return &AESGCM{}, nil
	}
	if len(key) != KeyLength {
		return nil, fmt.Errorf("key must be %d bytes, got %d", KeyLength, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &AESGCM{aead: aead}, nil
}

// Enabled reports whether a key was configured.
func (s *AESGCM) Enabled() bool { return s.aead != nil }

// Seal encrypts one value under a fresh nonce.
func (s *AESGCM) Seal(plaintext string) (string, error) {
	if s.aead == nil {
		return "", ErrNoKey
	}
	nonce := make([]byte, s.aead.NonceSize(), s.aead.NonceSize()+len(plaintext)+s.aead.Overhead())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := s.aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return envelopePrefix + base64.RawURLEncoding.EncodeToString(sealed), nil
}

// Open decrypts a value Seal wrote. A value that was never sealed is refused rather than handed
// back, so a plaintext credential cannot pass for a decrypted one.
func (s *AESGCM) Open(sealed string) (string, error) {
	if s.aead == nil {
		return "", ErrNoKey
	}
	body, found := strings.CutPrefix(sealed, envelopePrefix)
	if !found {
		return "", ErrMalformed
	}
	raw, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return "", ErrMalformed
	}
	if len(raw) < s.aead.NonceSize() {
		return "", ErrMalformed
	}
	plaintext, err := s.aead.Open(nil, raw[:s.aead.NonceSize()], raw[s.aead.NonceSize():], nil)
	if err != nil {
		return "", ErrMalformed
	}
	return string(plaintext), nil
}
