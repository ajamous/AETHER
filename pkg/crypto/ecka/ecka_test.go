package ecka

import (
	"bytes"
	"errors"
	"testing"
)

func TestECKA_AgreeP256(t *testing.T) {
	a, err := GenerateKeyPair(CurveP256)
	if err != nil {
		t.Fatalf("a: %v", err)
	}
	b, err := GenerateKeyPair(CurveP256)
	if err != nil {
		t.Fatalf("b: %v", err)
	}

	info := []byte("aether ecka test context")

	keyA, err := Derive(a.Priv, b.Pub, info, 32)
	if err != nil {
		t.Fatalf("derive A: %v", err)
	}
	keyB, err := Derive(b.Priv, a.Pub, info, 32)
	if err != nil {
		t.Fatalf("derive B: %v", err)
	}

	if !bytes.Equal(keyA, keyB) {
		t.Fatalf("ECKA keys do not match:\n  A=%x\n  B=%x", keyA, keyB)
	}
	if len(keyA) != 32 {
		t.Fatalf("expected 32 bytes, got %d", len(keyA))
	}
}

func TestECKA_DifferentInfoDifferentKey(t *testing.T) {
	a, err := GenerateKeyPair(CurveP256)
	if err != nil {
		t.Fatalf("a: %v", err)
	}
	b, err := GenerateKeyPair(CurveP256)
	if err != nil {
		t.Fatalf("b: %v", err)
	}

	k1, err := Derive(a.Priv, b.Pub, []byte("ctx-1"), 32)
	if err != nil {
		t.Fatalf("k1: %v", err)
	}
	k2, err := Derive(a.Priv, b.Pub, []byte("ctx-2"), 32)
	if err != nil {
		t.Fatalf("k2: %v", err)
	}
	if bytes.Equal(k1, k2) {
		t.Fatal("different shared-info should yield different keys")
	}
}

func TestECKA_BrainpoolNotImplemented(t *testing.T) {
	_, err := GenerateKeyPair(CurveBrainpoolP256r1)
	if !errors.Is(err, ErrBrainpoolNotImplemented) {
		t.Fatalf("expected ErrBrainpoolNotImplemented, got %v", err)
	}
}

func TestECKA_NilKeyRejected(t *testing.T) {
	if _, err := Derive(nil, nil, nil, 32); err == nil {
		t.Fatal("expected error on nil keys")
	}
}
