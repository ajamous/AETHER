package bpp

import (
	"bytes"
	"crypto/rand"
	"testing"

	"github.com/ajamous/aether/pkg/crypto/ecka"
)

// fixedKeys returns a SessionKeys whose three slots are
// deterministic so a test that wants byte-stable outputs has a
// known starting point.
func fixedKeys() *SessionKeys {
	return &SessionKeys{
		SENC:       bytes.Repeat([]byte{0x10}, 16),
		SMAC:       bytes.Repeat([]byte{0x20}, 16),
		InitialMCV: bytes.Repeat([]byte{0x30}, 16),
	}
}

func TestSealSegments_RoundTrip(t *testing.T) {
	keys := fixedKeys()
	plain := bytes.Repeat([]byte("the quick brown fox jumps over the lazy dog\n"), 30) // 1320 bytes — spans 2 segments at 1024
	segs, err := SealSegments(keys, plain, 1024)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if len(segs) != 2 {
		t.Fatalf("got %d segments, want 2 (1320 bytes / 1024 per segment)", len(segs))
	}
	got, err := OpenSegments(keys, segs)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if !bytes.Equal(got, plain) {
		t.Errorf("round-trip plaintext differs (len got=%d want=%d)", len(got), len(plain))
	}
}

func TestSealSegments_SmallPlaintext(t *testing.T) {
	keys := fixedKeys()
	// One byte of plaintext still produces one segment with a
	// 16-byte tag → 17 bytes on the wire.
	segs, err := SealSegments(keys, []byte{0xFF}, 1024)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if len(segs) != 1 {
		t.Fatalf("got %d segments, want 1", len(segs))
	}
	if len(segs[0]) != 17 {
		t.Errorf("segment len = %d, want 17 (1 byte plaintext + 16 byte tag)", len(segs[0]))
	}
	got, err := OpenSegments(keys, segs)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if !bytes.Equal(got, []byte{0xFF}) {
		t.Errorf("plaintext mismatch: %x", got)
	}
}

func TestSealSegments_BothSidesAgreeFromECKA(t *testing.T) {
	// End-to-end realism check: derive the keys from a fresh
	// ECKA exchange (matching how production will look), then
	// confirm both sides round-trip the same plaintext.
	spDP, _ := ecka.GenerateKeyPair(ecka.CurveP256)
	euicc, _ := ecka.GenerateKeyPair(ecka.CurveP256)
	sharedInfo := []byte("aether/test/sharedinfo")

	smdpKeys, err := Derive(spDP.Priv, euicc.Pub, sharedInfo)
	if err != nil {
		t.Fatalf("smdp derive: %v", err)
	}
	euiccKeys, err := Derive(euicc.Priv, spDP.Pub, sharedInfo)
	if err != nil {
		t.Fatalf("euicc derive: %v", err)
	}

	plain := make([]byte, 2400)
	if _, err := rand.Read(plain); err != nil {
		t.Fatalf("rand: %v", err)
	}

	// SM-DP+ seals.
	segs, err := SealSegments(smdpKeys, plain, 512)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	// eUICC opens with its independently-derived keys.
	got, err := OpenSegments(euiccKeys, segs)
	if err != nil {
		t.Fatalf("eUICC-side open: %v", err)
	}
	if !bytes.Equal(got, plain) {
		t.Error("eUICC-side plaintext does not match SM-DP+ input — keys disagree")
	}
}

func TestOpenSegments_DetectsTamperedCiphertext(t *testing.T) {
	keys := fixedKeys()
	plain := bytes.Repeat([]byte{0xAB}, 200)
	segs, _ := SealSegments(keys, plain, 1024)

	// Flip a byte in the middle of the ciphertext (not the tag).
	segs[0][0] ^= 0x80
	if _, err := OpenSegments(keys, segs); err == nil {
		t.Fatal("OpenSegments must reject tampered ciphertext")
	}
}

func TestOpenSegments_DetectsTamperedTag(t *testing.T) {
	keys := fixedKeys()
	plain := bytes.Repeat([]byte{0xAB}, 200)
	segs, _ := SealSegments(keys, plain, 1024)

	// Flip the last byte (part of the GCM tag).
	segs[0][len(segs[0])-1] ^= 0x01
	if _, err := OpenSegments(keys, segs); err == nil {
		t.Fatal("OpenSegments must reject tampered GCM tag")
	}
}

func TestOpenSegments_DetectsBrokenChain(t *testing.T) {
	// SCP03t-style MAC chaining means swapping segment order
	// breaks every segment after the swap point. Verifies
	// reordered segments are rejected — replay/permutation
	// defense.
	keys := fixedKeys()
	plain := bytes.Repeat([]byte("xyz"), 1000) // ~3000 bytes → 3+ segments at 1024
	segs, err := SealSegments(keys, plain, 1024)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if len(segs) < 2 {
		t.Fatalf("test needs ≥2 segments to swap; got %d", len(segs))
	}
	// Swap segments 0 and 1.
	segs[0], segs[1] = segs[1], segs[0]
	if _, err := OpenSegments(keys, segs); err == nil {
		t.Fatal("OpenSegments must reject reordered segments — MCV chain is broken")
	}
}

func TestSealSegments_Validation(t *testing.T) {
	keys := fixedKeys()
	cases := []struct {
		name        string
		keys        *SessionKeys
		plain       []byte
		segmentSize int
		wantErr     string
	}{
		{"nil keys", nil, []byte{0x01}, 1024, "nil keys"},
		{"short SENC", &SessionKeys{SENC: []byte{0x01}, InitialMCV: keys.InitialMCV}, []byte{0x01}, 1024, "SENC"},
		{"short MCV", &SessionKeys{SENC: keys.SENC, InitialMCV: []byte{0x01}}, []byte{0x01}, 1024, "InitialMCV"},
		{"zero segment size", keys, []byte{0x01}, 0, "segmentSize"},
		{"oversized segment size", keys, []byte{0x01}, MaxSegmentSize + 1, "segmentSize"},
		{"empty plaintext", keys, []byte{}, 1024, "empty plaintext"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := SealSegments(c.keys, c.plain, c.segmentSize)
			if err == nil {
				t.Fatalf("expected error containing %q", c.wantErr)
			}
			if !bytes.Contains([]byte(err.Error()), []byte(c.wantErr)) {
				t.Errorf("error %q did not mention %q", err.Error(), c.wantErr)
			}
		})
	}
}

func TestOpenSegments_Validation(t *testing.T) {
	keys := fixedKeys()
	if _, err := OpenSegments(nil, [][]byte{{0x00}}); err == nil {
		t.Error("nil keys must fail")
	}
	if _, err := OpenSegments(keys, nil); err == nil {
		t.Error("no segments must fail")
	}
	if _, err := OpenSegments(keys, [][]byte{{0x00, 0x01}}); err == nil {
		t.Error("segment shorter than tag length must fail")
	}
}

// TestSealSegments_CountersAreUnique is a paranoid check that two
// segments under the same SENC use different nonces. SCP03t
// inherits AES-GCM's nonce-reuse catastrophe — two segments with
// the same (SENC, nonce) pair would let an attacker recover
// SMAC-equivalent material from the XOR. The counter starts at 1
// and increments per segment; this test confirms.
func TestSealSegments_CountersAreUnique(t *testing.T) {
	keys := fixedKeys()
	// Identical 64-byte segments. If the counter wasn't being
	// incremented, both would produce identical ciphertext.
	plain := bytes.Repeat([]byte{0x42}, 128) // exactly two 64-byte segments
	segs, err := SealSegments(keys, plain, 64)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if len(segs) != 2 {
		t.Fatalf("got %d segments, want 2", len(segs))
	}
	if bytes.Equal(segs[0], segs[1]) {
		t.Fatal("two segments of identical plaintext produced identical ciphertext — nonce reuse")
	}
}
