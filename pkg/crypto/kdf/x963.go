// Package kdf implements the key derivation functions used by SGP.22.
//
// The KDF that matters for ECKA is X9.63 with SHA-256, specified in
// ANSI X9.63 §3.6.1 and referenced by SGP.22 §2.6.4.
package kdf

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"hash"
)

// X963 derives keyLen bytes from sharedSecret and sharedInfo using
// the X9.63 KDF with the given hash. See ANSI X9.63 §3.6.1; SGP.22
// §2.6.4 mandates SHA-256 for ECKA. Use X963SHA256 for that case.
//
// The function returns an error if keyLen exceeds (2^32 - 1) * hashLen,
// matching the upper bound the X9.63 spec sets.
func X963(h func() hash.Hash, sharedSecret, sharedInfo []byte, keyLen int) ([]byte, error) {
	if keyLen <= 0 {
		return nil, errors.New("kdf: keyLen must be positive")
	}
	hashLen := h().Size()
	const maxCounter uint64 = 1<<32 - 1
	if uint64(keyLen) > maxCounter*uint64(hashLen) { //nolint:gosec // keyLen checked > 0 above; on a 64-bit int the conversion is lossless
		return nil, errors.New("kdf: requested keyLen exceeds X9.63 maximum")
	}

	out := make([]byte, 0, keyLen)
	hasher := h()
	var counter uint32 = 1
	for len(out) < keyLen {
		hasher.Reset()
		hasher.Write(sharedSecret)
		var ctrBuf [4]byte
		binary.BigEndian.PutUint32(ctrBuf[:], counter)
		hasher.Write(ctrBuf[:])
		hasher.Write(sharedInfo)
		out = hasher.Sum(out)
		counter++
	}
	return out[:keyLen], nil
}

// X963SHA256 is X963 with SHA-256, the variant SGP.22 §2.6.4 calls for.
func X963SHA256(sharedSecret, sharedInfo []byte, keyLen int) ([]byte, error) {
	return X963(sha256.New, sharedSecret, sharedInfo, keyLen)
}
