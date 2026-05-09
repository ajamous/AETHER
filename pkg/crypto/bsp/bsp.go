// Package bsp implements the Bound Profile Protection primitives used
// by SGP.22 ES8+ to protect profile package data between the SM-DP+
// and the eUICC.
//
// SGP.22 §2.6 defines BSP as a layered scheme:
//
//   - An ECKA key agreement (see pkg/crypto/ecka) establishes session
//     key material between SM-DP+ and eUICC.
//   - A SCP03t-derived AES-128-GCM construction protects each
//     ProtectedProfilePackage block (SGP.22 §2.6.3, §5.5.4).
//
// This package provides the symmetric AES-128-GCM piece. The ECKA and
// SCP03t-style key derivation lives in pkg/crypto/ecka and the
// session-key labelling in services/smdp-plus.
//
// What this package is NOT: a complete SGP.22-compliant ProtectedProfile
// Package codec. The framing — segment counters, MAC chaining over
// segments, the specific shared-info bytes — lives where it has access
// to the surrounding ASN.1. We expose the cryptographic core; callers
// own the framing.
package bsp

import (
	"crypto/aes"
	"crypto/cipher"
	"errors"
	"fmt"
)

// AES-128-GCM is the AEAD SGP.22 §2.6.3 specifies for ES8+ payload
// protection when SCP03t is selected. We expose only the 128-bit key
// length to match the spec rather than offering the broader Go AES
// interface.
const (
	KeyLen   = 16 // bytes (AES-128)
	NonceLen = 12 // bytes (GCM standard nonce)
	TagLen   = 16 // bytes (GCM full-length tag, matches SCP03t MAC length)
)

// Encrypt seals plaintext with key under nonce, authenticating
// associatedData. Returns ciphertext || tag, the standard Go GCM
// shape, with len(out) == len(plaintext) + TagLen.
func Encrypt(key, nonce, plaintext, associatedData []byte) ([]byte, error) {
	a, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	if len(nonce) != NonceLen {
		return nil, fmt.Errorf("bsp: nonce must be %d bytes, got %d", NonceLen, len(nonce))
	}
	return a.Seal(nil, nonce, plaintext, associatedData), nil
}

// Decrypt opens ciphertext with key under nonce, verifying the MAC
// over associatedData. ciphertext must be ciphertext || tag, the
// standard Go GCM shape.
func Decrypt(key, nonce, ciphertext, associatedData []byte) ([]byte, error) {
	a, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	if len(nonce) != NonceLen {
		return nil, fmt.Errorf("bsp: nonce must be %d bytes, got %d", NonceLen, len(nonce))
	}
	pt, err := a.Open(nil, nonce, ciphertext, associatedData)
	if err != nil {
		return nil, fmt.Errorf("bsp: decrypt/authenticate: %w", err)
	}
	return pt, nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	if len(key) != KeyLen {
		return nil, fmt.Errorf("bsp: key must be %d bytes, got %d", KeyLen, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("bsp: aes: %w", err)
	}
	return cipher.NewGCM(block)
}

// SplitMACKey is a placeholder for the SCP03t-style key splitting that
// SGP.22 §2.6.3 specifies (S-ENC, S-MAC, S-RMAC, etc., derived via the
// ECKA-produced master). Callers wanting the full SCP03t shape should
// derive using pkg/crypto/ecka with the protocol-specific shared-info
// strings; that work lives in services/smdp-plus alongside the session
// state machine.
//
// This stub exists so the symbol shape is reserved; landing it where
// tests can lock the contract is a Phase 1 task on smdp-plus.
var ErrSCP03tNotImplemented = errors.New("bsp: SCP03t key splitting lives in services/smdp-plus")
