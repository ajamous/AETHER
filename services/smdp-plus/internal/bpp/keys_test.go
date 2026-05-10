package bpp

import (
	"bytes"
	"testing"

	"github.com/ajamous/aether/pkg/crypto/ecka"
)

func TestDerive_BothSidesAgree(t *testing.T) {
	// Generate two independent ECKA keypairs to stand in for
	// SM-DP+ and eUICC ephemeral keys. Both sides should derive
	// the same SessionKeys for the same sharedInfo — that's the
	// invariant the eUICC relies on to decrypt every BPP segment.
	spDP, err := ecka.GenerateKeyPair(ecka.CurveP256)
	if err != nil {
		t.Fatalf("spDP keygen: %v", err)
	}
	euicc, err := ecka.GenerateKeyPair(ecka.CurveP256)
	if err != nil {
		t.Fatalf("euicc keygen: %v", err)
	}
	sharedInfo := []byte("aether/test/sharedinfo/v1")

	smdp, err := Derive(spDP.Priv, euicc.Pub, sharedInfo)
	if err != nil {
		t.Fatalf("smdp derive: %v", err)
	}
	euiccSide, err := Derive(euicc.Priv, spDP.Pub, sharedInfo)
	if err != nil {
		t.Fatalf("euicc derive: %v", err)
	}

	if !bytes.Equal(smdp.SENC, euiccSide.SENC) {
		t.Errorf("SENC mismatch — eUICC will reject every segment")
	}
	if !bytes.Equal(smdp.SMAC, euiccSide.SMAC) {
		t.Errorf("SMAC mismatch")
	}
	if !bytes.Equal(smdp.InitialMCV, euiccSide.InitialMCV) {
		t.Errorf("InitialMCV mismatch")
	}
}

func TestDerive_SliceLengths(t *testing.T) {
	spDP, _ := ecka.GenerateKeyPair(ecka.CurveP256)
	euicc, _ := ecka.GenerateKeyPair(ecka.CurveP256)
	keys, err := Derive(spDP.Priv, euicc.Pub, []byte("ctx"))
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	if len(keys.SENC) != 16 {
		t.Errorf("SENC = %d bytes, want 16 (AES-128)", len(keys.SENC))
	}
	if len(keys.SMAC) != 16 {
		t.Errorf("SMAC = %d bytes, want 16", len(keys.SMAC))
	}
	if len(keys.InitialMCV) != 16 {
		t.Errorf("InitialMCV = %d bytes, want 16", len(keys.InitialMCV))
	}
}

func TestDerive_DistinctSlices(t *testing.T) {
	// SENC, SMAC, MCV must come from non-overlapping bytes of the
	// KDF output. If the slicing accidentally aliases (e.g. all
	// three pointing at material[0:16]), every byte would be
	// equal — catch that explicitly.
	spDP, _ := ecka.GenerateKeyPair(ecka.CurveP256)
	euicc, _ := ecka.GenerateKeyPair(ecka.CurveP256)
	keys, _ := Derive(spDP.Priv, euicc.Pub, []byte("ctx"))

	if bytes.Equal(keys.SENC, keys.SMAC) {
		t.Error("SENC and SMAC are identical — slicing is wrong")
	}
	if bytes.Equal(keys.SMAC, keys.InitialMCV) {
		t.Error("SMAC and InitialMCV are identical — slicing is wrong")
	}
	if bytes.Equal(keys.SENC, keys.InitialMCV) {
		t.Error("SENC and InitialMCV are identical — slicing is wrong")
	}
}

func TestDerive_DifferentSharedInfoDifferentKeys(t *testing.T) {
	// Catches the SGP.22 footgun where two protocol contexts
	// re-use the same ephemeral keypairs. Different sharedInfo
	// MUST produce different session keys, otherwise an attacker
	// who can replay traffic from one context into another
	// inherits a valid key.
	spDP, _ := ecka.GenerateKeyPair(ecka.CurveP256)
	euicc, _ := ecka.GenerateKeyPair(ecka.CurveP256)

	a, _ := Derive(spDP.Priv, euicc.Pub, []byte("context-A"))
	b, _ := Derive(spDP.Priv, euicc.Pub, []byte("context-B"))

	if bytes.Equal(a.SENC, b.SENC) {
		t.Error("Different sharedInfo produced identical SENC — KDF context binding is broken")
	}
	if bytes.Equal(a.SMAC, b.SMAC) {
		t.Error("Different sharedInfo produced identical SMAC")
	}
	if bytes.Equal(a.InitialMCV, b.InitialMCV) {
		t.Error("Different sharedInfo produced identical InitialMCV")
	}
}

func TestDerive_NilArgsRejected(t *testing.T) {
	spDP, _ := ecka.GenerateKeyPair(ecka.CurveP256)
	euicc, _ := ecka.GenerateKeyPair(ecka.CurveP256)
	if _, err := Derive(nil, euicc.Pub, []byte("ctx")); err == nil {
		t.Error("nil spDPpriv must be rejected")
	}
	if _, err := Derive(spDP.Priv, nil, []byte("ctx")); err == nil {
		t.Error("nil euiccPub must be rejected")
	}
}

func TestDerive_EmptySharedInfoStillDerives(t *testing.T) {
	// SGP.22 §H.4 sharedInfo always has content, but at the
	// crypto layer empty sharedInfo is a valid X9.63-KDF input.
	// Reject only NIL; allow empty.
	spDP, _ := ecka.GenerateKeyPair(ecka.CurveP256)
	euicc, _ := ecka.GenerateKeyPair(ecka.CurveP256)
	keys, err := Derive(spDP.Priv, euicc.Pub, []byte{})
	if err != nil {
		t.Fatalf("derive with empty sharedInfo: %v", err)
	}
	if len(keys.SENC) != 16 {
		t.Errorf("SENC = %d bytes", len(keys.SENC))
	}
}

// TestDerive_StableAcrossInvocations confirms two derivations on
// the same inputs are byte-identical. SCP03t requires this so the
// SM-DP+ and eUICC compute the same MCV chain on every BPP
// segment.
func TestDerive_StableAcrossInvocations(t *testing.T) {
	spDP, _ := ecka.GenerateKeyPair(ecka.CurveP256)
	euicc, _ := ecka.GenerateKeyPair(ecka.CurveP256)

	a, _ := Derive(spDP.Priv, euicc.Pub, []byte("ctx"))
	b, _ := Derive(spDP.Priv, euicc.Pub, []byte("ctx"))

	if !bytes.Equal(a.SENC, b.SENC) || !bytes.Equal(a.SMAC, b.SMAC) || !bytes.Equal(a.InitialMCV, b.InitialMCV) {
		t.Error("Derive is not deterministic for the same inputs")
	}
}
