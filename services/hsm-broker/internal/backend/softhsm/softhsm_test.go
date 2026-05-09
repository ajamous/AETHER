package softhsm

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/asn1"
	"math/big"
	"os"
	"strconv"
	"testing"

	hsmv1 "github.com/ajamous/aether/services/hsm-broker/api/v1"
)

var cryptoRandRead = rand.Read

// SoftHSM integration tests.
//
// These exercise the real PKCS#11 path. They skip unless the
// environment has SoftHSM v2 installed and a token initialized:
//
//	export AETHER_SOFTHSM_LIB=/usr/lib/softhsm/libsofthsm2.so
//	export AETHER_SOFTHSM_SLOT=<slot id from `softhsm2-util --show-slots`>
//	export AETHER_SOFTHSM_PIN=1234
//	export SOFTHSM2_CONF=/path/to/softhsm2.conf
//	go test ./internal/backend/softhsm/...
//
// `make softhsm-init` (forthcoming) sets these up against a temp
// token directory in one command.

func openOrSkip(t *testing.T) *Backend {
	t.Helper()
	lib := os.Getenv("AETHER_SOFTHSM_LIB")
	pin := os.Getenv("AETHER_SOFTHSM_PIN")
	slotStr := os.Getenv("AETHER_SOFTHSM_SLOT")
	if lib == "" || pin == "" || slotStr == "" {
		t.Skip("SoftHSM env not set (AETHER_SOFTHSM_LIB / SLOT / PIN); skipping integration test")
	}
	slot, err := strconv.ParseUint(slotStr, 10, 32)
	if err != nil {
		t.Fatalf("AETHER_SOFTHSM_SLOT: %v", err)
	}
	b, err := New(Config{LibraryPath: lib, Slot: uint(slot), PIN: pin})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() {
		if err := b.Close(); err != nil {
			t.Logf("close: %v", err)
		}
	})
	return b
}

func TestSoftHSM_Health(t *testing.T) {
	b := openOrSkip(t)
	resp, err := b.Health(context.Background())
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	if !resp.Ready || resp.Backend != "softhsm" {
		t.Fatalf("unexpected: %+v", resp)
	}
}

func TestSoftHSM_GenerateAndSignAndVerify(t *testing.T) {
	b := openOrSkip(t)
	ctx := context.Background()

	gen, err := b.GenerateKeyPair(ctx, &hsmv1.GenerateKeyPairRequest{
		Label: "aether-test-ecdsa-" + uniq(),
		Kind:  hsmv1.KeyKindECDSA,
		Curve: hsmv1.CurveP256,
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if gen.Handle.ID == "" {
		t.Fatal("expected non-empty key id")
	}
	if len(gen.PublicKey) == 0 || gen.PublicKey[0] != 0x04 {
		t.Fatalf("public key not uncompressed point, got prefix %x", gen.PublicKey[:1])
	}

	digest := sha256.Sum256([]byte("aether-softhsm-roundtrip"))
	sig, err := b.Sign(ctx, &hsmv1.SignRequest{
		KeyID:     gen.Handle.ID,
		Digest:    digest[:],
		DigestAlg: hsmv1.HashSHA256,
	})
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	// Verify the signature client-side using stdlib ECDSA against the
	// public key we just got back. This proves the SoftHSM round-trip
	// is correct end-to-end, not just that something came back.
	x, y := elliptic.Unmarshal(elliptic.P256(), gen.PublicKey)
	if x == nil {
		t.Fatal("failed to unmarshal public key")
	}
	pub := &ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y}
	var parsed struct{ R, S *big.Int }
	if _, err := asn1.Unmarshal(sig.SignatureDER, &parsed); err != nil {
		t.Fatalf("parse signature: %v", err)
	}
	if !ecdsa.Verify(pub, digest[:], parsed.R, parsed.S) {
		t.Fatal("signature did not verify against returned public key")
	}
}

func TestSoftHSM_DeriveKey_ECKA(t *testing.T) {
	b := openOrSkip(t)
	ctx := context.Background()

	a, err := b.GenerateKeyPair(ctx, &hsmv1.GenerateKeyPairRequest{
		Label: "aether-test-ecka-A-" + uniq(),
		Kind:  hsmv1.KeyKindECKA,
		Curve: hsmv1.CurveP256,
	})
	if err != nil {
		t.Fatalf("gen A: %v", err)
	}

	// Generate a peer keypair *outside* the HSM (in Go) so we can
	// simulate an LPA's eUICC presenting its public point.
	peerPriv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ecdh peer keygen: %v", err)
	}
	peerPub := peerPriv.PublicKey().Bytes() // uncompressed X9.63 point

	derived, err := b.DeriveKey(ctx, &hsmv1.DeriveKeyRequest{
		KeyID:      a.Handle.ID,
		PeerPublic: peerPub,
		SharedInfo: []byte("aether-test-context"),
		KeyLen:     32,
	})
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	if derived.SessionKeyID == "" {
		t.Fatal("expected session_key_id")
	}

	// We deliberately do NOT extract the secret bytes — that's the whole
	// point. A real production path uses the handle for subsequent
	// AES ops. Confirm the handle exists by listing.
	list, err := b.ListKeys(ctx, &hsmv1.ListKeysRequest{LabelPrefix: "aether-test-ecka-A"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list.Keys) == 0 {
		t.Fatal("expected at least one matching key")
	}
}

func TestSoftHSM_ListKeys_LabelPrefixFilter(t *testing.T) {
	b := openOrSkip(t)
	ctx := context.Background()

	prefix := "aether-test-list-" + uniq() + "-"
	for i := 0; i < 3; i++ {
		_, err := b.GenerateKeyPair(ctx, &hsmv1.GenerateKeyPairRequest{
			Label: prefix + strconv.Itoa(i),
			Kind:  hsmv1.KeyKindECDSA,
			Curve: hsmv1.CurveP256,
		})
		if err != nil {
			t.Fatalf("gen %d: %v", i, err)
		}
	}
	resp, err := b.ListKeys(ctx, &hsmv1.ListKeysRequest{LabelPrefix: prefix})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(resp.Keys) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(resp.Keys))
	}
}

func TestSoftHSM_SignKeyNotFound(t *testing.T) {
	b := openOrSkip(t)
	_, err := b.Sign(context.Background(), &hsmv1.SignRequest{
		KeyID:     "no-such-key-aaaaaaaaaaaaaaaaaaaaaaaa",
		Digest:    bytes.Repeat([]byte{0}, 32),
		DigestAlg: hsmv1.HashSHA256,
	})
	if err == nil {
		t.Fatal("expected error on missing key")
	}
}

// uniq returns a short hex string so parallel test runs against the
// same SoftHSM token don't collide on labels.
func uniq() string {
	var b [4]byte
	_, _ = cryptoRandRead(b[:])
	const hexc = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, v := range b {
		out[i*2] = hexc[v>>4]
		out[i*2+1] = hexc[v&0xF]
	}
	return string(out)
}
