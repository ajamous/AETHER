// SCP03t-style segmentation with MAC chaining.
//
// BPP delivers the SAIP UPP to the eUICC as a sequence of
// AES-128-GCM-sealed segments. SGP.22 §2.6.3 specifies SCP03t as
// the AEAD construction: each segment is encrypted under SENC, the
// 128-bit GCM tag is the MAC, and the previous segment's tag feeds
// into the next segment's AAD ("MAC chaining vector" / MCV).
//
// What this file ships
//
// `SealSegments` takes the SAIP UPP plaintext + the SessionKeys from
// `Derive` and returns one ciphertext-with-tag slice per segment,
// in order, with the chain wired so the eUICC can verify each
// segment in turn. `OpenSegments` reverses the operation under the
// same keys — used by tests today, and by an audit/replay tool in
// the future.
//
// What this file does NOT ship
//
// The exact byte layout SGP.22 §H.3 specifies for the per-segment
// AAD (counter encoding, tag bits, ICV-as-AAD framing) is more
// than this file's "chunked GCM with chained tags" model. The
// model here matches the SCP03t-style chain in *shape* —
// segment-by-segment encryption with MAC chaining — but the wire
// bytes a real eUICC accepts will need the spec-precise AAD
// construction, which lands as a follow-up refinement once the
// hardware bench (a sysmoEUICC1-C2T card) catches the first
// mismatch. Until that bench is online, this file's tests round-
// trip Seal/Open against themselves; the cross-vendor interop
// claim is honestly out of scope.
package bpp

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"errors"
	"fmt"
)

// MaxSegmentSize is the conservative upper bound on the plaintext
// length of a single BPP segment. SGP.22 mandates at most 1024
// payload bytes per segment for SCP03t; we expose this rather than
// a default so callers can pick a tighter limit if their eUICC is
// known to be more constrained.
const MaxSegmentSize = 1024

// segmentNonce builds a 12-byte GCM nonce from a session counter.
// SCP03t uses a deterministic counter rather than a random nonce —
// every (SENC, counter) pair is unique because SENC is fresh per
// session. Layout: 4 zero bytes || big-endian uint64 counter.
func segmentNonce(counter uint64) []byte {
	var n [12]byte
	binary.BigEndian.PutUint64(n[4:], counter)
	return n[:]
}

// SealSegments splits plaintext into chunks of at most segmentSize
// bytes, AES-128-GCM-seals each chunk under keys.SENC, and chains
// the previous segment's tag into the next segment's AAD via
// keys.InitialMCV. Returns one ciphertext-with-tag slice per
// segment in order.
//
// The first segment's AAD is keys.InitialMCV. Subsequent segments'
// AAD is the previous segment's 16-byte GCM tag. This is the
// mechanical chain SCP03t describes; spec-precise per-segment AAD
// framing is the follow-up refinement called out in this file's
// godoc.
//
// segmentSize must be in (0, MaxSegmentSize]; values outside that
// range return an error rather than truncating silently.
func SealSegments(keys *SessionKeys, plaintext []byte, segmentSize int) ([][]byte, error) {
	if keys == nil {
		return nil, errors.New("bpp: SealSegments: nil keys")
	}
	if len(keys.SENC) != sencLen {
		return nil, fmt.Errorf("bpp: SealSegments: SENC len %d, want %d", len(keys.SENC), sencLen)
	}
	if len(keys.InitialMCV) != mcvLen {
		return nil, fmt.Errorf("bpp: SealSegments: InitialMCV len %d, want %d", len(keys.InitialMCV), mcvLen)
	}
	if segmentSize <= 0 || segmentSize > MaxSegmentSize {
		return nil, fmt.Errorf("bpp: SealSegments: segmentSize %d outside (0, %d]", segmentSize, MaxSegmentSize)
	}

	gcm, err := newGCM(keys.SENC)
	if err != nil {
		return nil, err
	}

	out := make([][]byte, 0, (len(plaintext)+segmentSize-1)/segmentSize)
	mcv := append([]byte(nil), keys.InitialMCV...)
	var counter uint64 = 1
	for off := 0; off < len(plaintext); off += segmentSize {
		end := off + segmentSize
		if end > len(plaintext) {
			end = len(plaintext)
		}
		segment := plaintext[off:end]
		// Seal returns ciphertext || tag (Go GCM convention). The
		// last 16 bytes are the tag we feed into the next segment's
		// MCV.
		sealed := gcm.Seal(nil, segmentNonce(counter), segment, mcv)
		out = append(out, sealed)

		// Update MCV to this segment's tag for the next round.
		mcv = append([]byte(nil), sealed[len(sealed)-tagLen:]...)
		counter++
	}
	// Empty plaintext produces zero segments. The eUICC won't accept
	// that — a UPP must have at least one segment — but reject it
	// at the caller layer rather than emit a no-op chain.
	if len(out) == 0 {
		return nil, errors.New("bpp: SealSegments: empty plaintext")
	}
	return out, nil
}

// OpenSegments reverses SealSegments. Returns the concatenated
// plaintext if every segment authenticates and the chain is
// consistent. On any failure (bad tag, broken chain, wrong key)
// returns an error and a nil slice — partial decryption is never
// returned, since SCP03t treats a chain break as a fatal session
// error.
func OpenSegments(keys *SessionKeys, segments [][]byte) ([]byte, error) {
	if keys == nil {
		return nil, errors.New("bpp: OpenSegments: nil keys")
	}
	if len(keys.SENC) != sencLen {
		return nil, fmt.Errorf("bpp: OpenSegments: SENC len %d, want %d", len(keys.SENC), sencLen)
	}
	if len(keys.InitialMCV) != mcvLen {
		return nil, fmt.Errorf("bpp: OpenSegments: InitialMCV len %d, want %d", len(keys.InitialMCV), mcvLen)
	}
	if len(segments) == 0 {
		return nil, errors.New("bpp: OpenSegments: no segments")
	}

	gcm, err := newGCM(keys.SENC)
	if err != nil {
		return nil, err
	}

	var out []byte
	mcv := append([]byte(nil), keys.InitialMCV...)
	var counter uint64 = 1
	for i, seg := range segments {
		if len(seg) < tagLen {
			return nil, fmt.Errorf("bpp: segment %d shorter than %d-byte tag", i, tagLen)
		}
		plaintext, err := gcm.Open(nil, segmentNonce(counter), seg, mcv)
		if err != nil {
			return nil, fmt.Errorf("bpp: segment %d: %w", i, err)
		}
		out = append(out, plaintext...)
		mcv = append([]byte(nil), seg[len(seg)-tagLen:]...)
		counter++
	}
	return out, nil
}

const tagLen = 16 // AES-GCM standard tag length, matches SCP03t

// newGCM is a tiny indirection so SealSegments and OpenSegments
// share one cipher-construction path. Centralised for fewer
// places that touch the AES-128 key.
func newGCM(key []byte) (cipher.AEAD, error) {
	if len(key) != aes.BlockSize {
		return nil, fmt.Errorf("bpp: SENC must be %d bytes, got %d", aes.BlockSize, len(key))
	}
	c, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("bpp: AES init: %w", err)
	}
	return cipher.NewGCM(c)
}
