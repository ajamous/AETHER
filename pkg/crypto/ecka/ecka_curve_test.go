package ecka

import (
	"bytes"
	"testing"

	"github.com/ajamous/aether/pkg/crypto/brainpool"
)

// agreementRoundTrip checks the defining ECKA property: two parties
// derive identical key material from each other's public key, and the
// derivation is sensitive to sharedInfo.
func agreementRoundTrip(t *testing.T, c Curve) {
	t.Helper()
	a, err := Generate(c)
	if err != nil {
		t.Fatalf("generate A: %v", err)
	}
	b, err := Generate(c)
	if err != nil {
		t.Fatalf("generate B: %v", err)
	}

	pubA := a.PublicBytes()
	pubB := b.PublicBytes()
	if len(pubA) != 65 || pubA[0] != 0x04 {
		t.Fatalf("public key not uncompressed X9.63: len=%d first=0x%02x", len(pubA), pubA[0])
	}

	info := []byte("aether ecka shared-info")
	za, err := a.DeriveBytes(pubB, info, 48)
	if err != nil {
		t.Fatalf("A derive: %v", err)
	}
	zb, err := b.DeriveBytes(pubA, info, 48)
	if err != nil {
		t.Fatalf("B derive: %v", err)
	}
	if !bytes.Equal(za, zb) {
		t.Fatalf("parties disagree:\n A=%x\n B=%x", za, zb)
	}
	if len(za) != 48 {
		t.Fatalf("derived %d bytes, want 48", len(za))
	}

	// Different sharedInfo must yield different material.
	zc, err := a.DeriveBytes(pubB, []byte("different"), 48)
	if err != nil {
		t.Fatalf("A derive 2: %v", err)
	}
	if bytes.Equal(za, zc) {
		t.Fatal("sharedInfo did not affect the derived key")
	}
}

func TestAgreementP256(t *testing.T)      { agreementRoundTrip(t, CurveP256) }
func TestAgreementBrainpool(t *testing.T) { agreementRoundTrip(t, CurveBrainpoolP256r1) }

func TestDeriveRejectsBadPeer(t *testing.T) {
	for _, c := range []Curve{CurveP256, CurveBrainpoolP256r1} {
		k, err := Generate(c)
		if err != nil {
			t.Fatalf("generate: %v", err)
		}
		cases := map[string][]byte{
			"empty":            {},
			"wrong length":     make([]byte, 64),
			"infinity":         append([]byte{0x04}, make([]byte, 64)...),
			"off curve / junk": bytes.Repeat([]byte{0x04}, 65),
		}
		for name, peer := range cases {
			if _, err := k.DeriveBytes(peer, nil, 32); err == nil {
				t.Errorf("curve %d: expected error for %s peer key", c, name)
			}
		}
	}
}

// TestBrainpoolKeyIsOnCurve confirms Generate produces a public point
// that actually lies on brainpoolP256r1 (guards the scalar-base-mult
// path through the curve-agnostic wrapper).
func TestBrainpoolKeyIsOnCurve(t *testing.T) {
	k, err := Generate(CurveBrainpoolP256r1)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !brainpool.IsOnCurve(k.bp.x, k.bp.y) {
		t.Fatal("generated Brainpool public key is not on the curve")
	}
	if k.Curve() != CurveBrainpoolP256r1 {
		t.Fatalf("Curve() = %d, want Brainpool", k.Curve())
	}
}

// TestLegacyKeyPairStillRejectsBrainpool documents that the old
// crypto/ecdh KeyPair API remains P-256 only.
func TestLegacyKeyPairStillRejectsBrainpool(t *testing.T) {
	if _, err := GenerateKeyPair(CurveBrainpoolP256r1); err == nil {
		t.Fatal("legacy GenerateKeyPair should reject Brainpool")
	}
}
