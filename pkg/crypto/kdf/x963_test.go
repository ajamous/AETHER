package kdf

import (
	"bytes"
	"encoding/hex"
	"testing"
)

// TestX963SHA256_NIST_CAVP exercises the X9.63 KDF against a published
// test vector from NIST CAVP "ANS X9.63 KDF" (SHA-256). The vector is:
//
//	Z (shared secret), shared-info, expected key
//
// This vector is from the NIST CAVP X9.63 KDF test set and is widely
// reproduced; if it ever fails after a change, the change is wrong, not
// the test.
func TestX963SHA256_NIST_CAVP(t *testing.T) {
	z, _ := hex.DecodeString("96c05619d56c328ab95fe84b18264b08725b85e33fd34f08")
	info, _ := hex.DecodeString("")
	want, _ := hex.DecodeString("443024c3dae66b95e6f5670601558f71")

	got, err := X963SHA256(z, info, len(want))
	if err != nil {
		t.Fatalf("kdf: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("kdf mismatch:\n  got  %x\n  want %x", got, want)
	}
}

func TestX963SHA256_LongerThanHash(t *testing.T) {
	z := []byte("shared-secret-bytes-32-aaaaaaaaaa")
	info := []byte("aether-test")

	out64, err := X963SHA256(z, info, 64)
	if err != nil {
		t.Fatalf("kdf 64: %v", err)
	}
	if len(out64) != 64 {
		t.Fatalf("expected 64 bytes, got %d", len(out64))
	}

	out32, err := X963SHA256(z, info, 32)
	if err != nil {
		t.Fatalf("kdf 32: %v", err)
	}
	if !bytes.Equal(out64[:32], out32) {
		t.Fatal("first 32 bytes of 64-byte derivation should match standalone 32-byte derivation")
	}
}

func TestX963SHA256_DifferentInfoDifferentKey(t *testing.T) {
	z := []byte("shared-secret-bytes-32-bbbbbbbbbb")
	a, err := X963SHA256(z, []byte("info-A"), 32)
	if err != nil {
		t.Fatalf("a: %v", err)
	}
	b, err := X963SHA256(z, []byte("info-B"), 32)
	if err != nil {
		t.Fatalf("b: %v", err)
	}
	if bytes.Equal(a, b) {
		t.Fatal("different shared-info should yield different keys")
	}
}

func TestX963_KeyLenZero(t *testing.T) {
	_, err := X963SHA256([]byte("z"), nil, 0)
	if err == nil {
		t.Fatal("expected error on keyLen=0")
	}
}
