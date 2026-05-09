package ecdsa

import (
	"crypto/rand"
	"errors"
	"testing"
)

func TestSignVerifyP256_RoundTrip(t *testing.T) {
	priv, err := GenerateKey(CurveP256, rand.Reader)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	msg := []byte("aether SGP.22 test message")
	sig, err := SignSHA256(priv, msg)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if err := VerifySHA256(&priv.PublicKey, msg, sig); err != nil {
		t.Fatalf("verify: %v", err)
	}
}

func TestVerifyP256_RejectsTamperedMessage(t *testing.T) {
	priv, err := GenerateKey(CurveP256, rand.Reader)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	msg := []byte("authentic")
	sig, err := SignSHA256(priv, msg)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	tampered := []byte("AUTHENTIC")
	if err := VerifySHA256(&priv.PublicKey, tampered, sig); err == nil {
		t.Fatal("expected verification failure on tampered message")
	}
}

func TestVerifyP256_RejectsTamperedSignature(t *testing.T) {
	priv, err := GenerateKey(CurveP256, rand.Reader)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	msg := []byte("authentic")
	sig, err := SignSHA256(priv, msg)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	sig[len(sig)-1] ^= 0x01
	if err := VerifySHA256(&priv.PublicKey, msg, sig); err == nil {
		t.Fatal("expected verification failure on tampered signature")
	}
}

func TestVerifyP256_RejectsTrailingBytes(t *testing.T) {
	priv, err := GenerateKey(CurveP256, rand.Reader)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	msg := []byte("authentic")
	sig, err := SignSHA256(priv, msg)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	sig = append(sig, 0xFF, 0xFF)
	if err := VerifySHA256(&priv.PublicKey, msg, sig); err == nil {
		t.Fatal("expected verification failure on trailing bytes")
	}
}

func TestBrainpoolP256r1_NotYetImplemented(t *testing.T) {
	_, err := GenerateKey(CurveBrainpoolP256r1, rand.Reader)
	if !errors.Is(err, ErrBrainpoolNotImplemented) {
		t.Fatalf("expected ErrBrainpoolNotImplemented, got %v", err)
	}
}

func TestUnknownCurve(t *testing.T) {
	_, err := GenerateKey(Curve(99), rand.Reader)
	if err == nil {
		t.Fatal("expected error on unknown curve")
	}
}

func TestSignSHA256_NilKey(t *testing.T) {
	if _, err := SignSHA256(nil, []byte("x")); err == nil {
		t.Fatal("expected error on nil private key")
	}
}

func TestVerifySHA256_NilKey(t *testing.T) {
	if err := VerifySHA256(nil, []byte("x"), []byte{0x30, 0x00}); err == nil {
		t.Fatal("expected error on nil public key")
	}
}
