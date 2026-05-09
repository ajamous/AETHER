package memory

import (
	"context"
	"crypto/sha256"
	"testing"

	hsmv1 "github.com/ajamous/aether/services/hsm-broker/api/v1"
)

func TestMemory_HealthAlwaysReady(t *testing.T) {
	b := New()
	resp, err := b.Health(context.Background())
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	if !resp.Ready {
		t.Fatal("memory backend should always be ready")
	}
	if resp.Backend != "memory" {
		t.Fatalf("backend name = %q, want memory", resp.Backend)
	}
}

func TestMemory_GenerateAndSignECDSA(t *testing.T) {
	b := New()
	ctx := context.Background()
	gen, err := b.GenerateKeyPair(ctx, &hsmv1.GenerateKeyPairRequest{
		Label: "DPpb-test",
		Kind:  hsmv1.KeyKindECDSA,
		Curve: hsmv1.CurveP256,
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if gen.Handle.ID == "" {
		t.Fatal("expected non-empty key id")
	}
	if len(gen.PublicKey) == 0 {
		t.Fatal("expected non-empty public key")
	}

	digest := sha256.Sum256([]byte("aether sign test"))
	sig, err := b.Sign(ctx, &hsmv1.SignRequest{
		KeyID:     gen.Handle.ID,
		Digest:    digest[:],
		DigestAlg: hsmv1.HashSHA256,
	})
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if len(sig.SignatureDER) == 0 {
		t.Fatal("expected non-empty signature")
	}
}

func TestMemory_DeriveKey_ECKA(t *testing.T) {
	b := New()
	ctx := context.Background()

	a, err := b.GenerateKeyPair(ctx, &hsmv1.GenerateKeyPairRequest{
		Label: "A",
		Kind:  hsmv1.KeyKindECKA,
		Curve: hsmv1.CurveP256,
	})
	if err != nil {
		t.Fatalf("gen A: %v", err)
	}
	bb, err := b.GenerateKeyPair(ctx, &hsmv1.GenerateKeyPairRequest{
		Label: "B",
		Kind:  hsmv1.KeyKindECKA,
		Curve: hsmv1.CurveP256,
	})
	if err != nil {
		t.Fatalf("gen B: %v", err)
	}

	info := []byte("aether-derive-test")
	derived, err := b.DeriveKey(ctx, &hsmv1.DeriveKeyRequest{
		KeyID:      a.Handle.ID,
		PeerPublic: bb.PublicKey,
		SharedInfo: info,
		KeyLen:     32,
	})
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	bytesA, err := b.SessionBytes(derived.SessionKeyID)
	if err != nil {
		t.Fatalf("session bytes A: %v", err)
	}

	derived2, err := b.DeriveKey(ctx, &hsmv1.DeriveKeyRequest{
		KeyID:      bb.Handle.ID,
		PeerPublic: a.PublicKey,
		SharedInfo: info,
		KeyLen:     32,
	})
	if err != nil {
		t.Fatalf("derive2: %v", err)
	}
	bytesB, err := b.SessionBytes(derived2.SessionKeyID)
	if err != nil {
		t.Fatalf("session bytes B: %v", err)
	}

	if string(bytesA) != string(bytesB) {
		t.Fatalf("derived keys differ:\n  A=%x\n  B=%x", bytesA, bytesB)
	}
}

func TestMemory_ListKeys_Filter(t *testing.T) {
	b := New()
	ctx := context.Background()
	for _, label := range []string{"DPtls", "DPauth", "DPpb", "ServiceFoo"} {
		_, _ = b.GenerateKeyPair(ctx, &hsmv1.GenerateKeyPairRequest{
			Label: label, Kind: hsmv1.KeyKindECDSA, Curve: hsmv1.CurveP256,
		})
	}

	all, err := b.ListKeys(ctx, &hsmv1.ListKeysRequest{})
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all.Keys) != 4 {
		t.Fatalf("expected 4 keys, got %d", len(all.Keys))
	}

	dpOnly, err := b.ListKeys(ctx, &hsmv1.ListKeysRequest{LabelPrefix: "DP"})
	if err != nil {
		t.Fatalf("list filtered: %v", err)
	}
	if len(dpOnly.Keys) != 3 {
		t.Fatalf("expected 3 DP* keys, got %d", len(dpOnly.Keys))
	}
}

func TestMemory_KeyNotFound(t *testing.T) {
	b := New()
	ctx := context.Background()
	_, err := b.Sign(ctx, &hsmv1.SignRequest{
		KeyID:     "nonexistent",
		Digest:    make([]byte, 32),
		DigestAlg: hsmv1.HashSHA256,
	})
	if err == nil {
		t.Fatal("expected ErrKeyNotFound on unknown key")
	}
}
