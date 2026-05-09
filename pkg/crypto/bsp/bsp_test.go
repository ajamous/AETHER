package bsp

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	key := make([]byte, KeyLen)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("key: %v", err)
	}
	nonce := make([]byte, NonceLen)
	if _, err := rand.Read(nonce); err != nil {
		t.Fatalf("nonce: %v", err)
	}
	pt := []byte("ProfileMetadata: ICCID=8901234567890123456")
	ad := []byte("aether-bsp-context")

	ct, err := Encrypt(key, nonce, pt, ad)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if len(ct) != len(pt)+TagLen {
		t.Fatalf("ciphertext length = %d, want %d", len(ct), len(pt)+TagLen)
	}
	out, err := Decrypt(key, nonce, ct, ad)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if !bytes.Equal(out, pt) {
		t.Fatalf("plaintext mismatch:\n  got  %q\n  want %q", out, pt)
	}
}

func TestDecrypt_RejectsTamperedCiphertext(t *testing.T) {
	key := make([]byte, KeyLen)
	rand.Read(key)
	nonce := make([]byte, NonceLen)
	rand.Read(nonce)

	ct, err := Encrypt(key, nonce, []byte("hello"), []byte("ctx"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	ct[0] ^= 0x01
	if _, err := Decrypt(key, nonce, ct, []byte("ctx")); err == nil {
		t.Fatal("expected decrypt failure on tampered ciphertext")
	}
}

func TestDecrypt_RejectsTamperedAssociatedData(t *testing.T) {
	key := make([]byte, KeyLen)
	rand.Read(key)
	nonce := make([]byte, NonceLen)
	rand.Read(nonce)

	ct, err := Encrypt(key, nonce, []byte("hello"), []byte("ctx"))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if _, err := Decrypt(key, nonce, ct, []byte("CTX")); err == nil {
		t.Fatal("expected decrypt failure on different associated data")
	}
}

func TestEncrypt_RejectsWrongKeyLen(t *testing.T) {
	if _, err := Encrypt([]byte("short"), make([]byte, NonceLen), []byte("x"), nil); err == nil {
		t.Fatal("expected error on short key")
	}
}

func TestEncrypt_RejectsWrongNonceLen(t *testing.T) {
	key := make([]byte, KeyLen)
	if _, err := Encrypt(key, []byte("short"), []byte("x"), nil); err == nil {
		t.Fatal("expected error on short nonce")
	}
}
