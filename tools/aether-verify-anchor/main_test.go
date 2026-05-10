package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fixture is a self-signed audit-anchor key + a freshly-signed
// JSON anchor — the same shape the audit service's /v1/anchor
// emits in production.
type fixture struct {
	priv    *ecdsa.PrivateKey
	pubPath string
	anchor  map[string]any
	tmpDir  string
}

func newFixture(t *testing.T, length int64, tailHash []byte) *fixture {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	tmpDir := t.TempDir()

	// Write the public key in PKIX PEM form — the format an
	// auditor would publish in the SAS-SM evidence pack.
	pubBytes, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatalf("marshal pub: %v", err)
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubBytes})
	pubPath := filepath.Join(tmpDir, "audit-anchor-pub.pem")
	if err := os.WriteFile(pubPath, pubPEM, 0o600); err != nil {
		t.Fatalf("write pub: %v", err)
	}

	// Build a signed anchor identical to what the audit service
	// would emit. Using the same anchorDER definition as main.go.
	now := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)
	type anchorDER struct {
		Timestamp time.Time `asn1:"generalized"`
		Length    int64
		TailHash  []byte
	}
	der, err := asn1.Marshal(anchorDER{Timestamp: now, Length: length, TailHash: tailHash})
	if err != nil {
		t.Fatalf("der marshal: %v", err)
	}
	digest := sha256.Sum256(der)
	rr, ss, err := ecdsa.Sign(rand.Reader, priv, digest[:])
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	sigDER, _ := asn1.Marshal(struct{ R, S *big.Int }{rr, ss})

	anchor := map[string]any{
		"length":         length,
		"tail_hash":      hex.EncodeToString(tailHash),
		"timestamp":      now.Format(time.RFC3339),
		"signed_payload": der,
		"signature":      sigDER,
		"signature_alg":  "ECDSA-SHA-256",
	}

	return &fixture{
		priv:    priv,
		pubPath: pubPath,
		anchor:  anchor,
		tmpDir:  tmpDir,
	}
}

func (f *fixture) writeAnchor(t *testing.T, mut func(map[string]any)) string {
	t.Helper()
	a := make(map[string]any, len(f.anchor))
	for k, v := range f.anchor {
		a[k] = v
	}
	if mut != nil {
		mut(a)
	}
	b, _ := json.Marshal(a)
	p := filepath.Join(f.tmpDir, "anchor.json")
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatalf("write anchor: %v", err)
	}
	return p
}

// runMain invokes the CLI's run() with a fresh argv, captures
// stdout/stderr (best effort), and returns the exit code.
func runMain(t *testing.T, args ...string) int {
	t.Helper()
	old := os.Args
	defer func() { os.Args = old }()
	os.Args = append([]string{"aether-verify-anchor"}, args...)
	// reset flag state between tests
	resetFlags()
	return run()
}

// resetFlags re-initialises the global flag.CommandLine so the
// CLI can be invoked multiple times in one test process.
func resetFlags() {
	// Tests share the global flag set; reset by replacing it.
	// flag.CommandLine is var-assignable.
	flagCommandLineReset()
}

func TestRun_HappyPath(t *testing.T) {
	fx := newFixture(t, 42, bytes.Repeat([]byte{0xAB}, 32))
	anchor := fx.writeAnchor(t, nil)

	if got := runMain(t, "--pubkey", fx.pubPath, "--anchor", anchor); got != exitOK {
		t.Fatalf("exit = %d, want %d (OK)", got, exitOK)
	}
}

func TestRun_MissingArgs(t *testing.T) {
	if got := runMain(t); got != exitBadInput {
		t.Errorf("no args: exit = %d, want %d", got, exitBadInput)
	}
}

func TestRun_TamperedSignedPayload(t *testing.T) {
	fx := newFixture(t, 42, bytes.Repeat([]byte{0xAB}, 32))
	anchor := fx.writeAnchor(t, func(a map[string]any) {
		// Replace the signed_payload with a fresh anchor whose
		// length/tail_hash differs from the JSON. The signature
		// is for the original payload — verify will reject.
		bad, _ := asn1.Marshal(struct {
			Timestamp time.Time `asn1:"generalized"`
			Length    int64
			TailHash  []byte
		}{Timestamp: time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC), Length: 99, TailHash: bytes.Repeat([]byte{0xCC}, 32)})
		a["signed_payload"] = bad
	})
	if got := runMain(t, "--pubkey", fx.pubPath, "--anchor", anchor); got != exitBadSig {
		t.Errorf("exit = %d, want %d (bad sig)", got, exitBadSig)
	}
}

func TestRun_TamperedSignature(t *testing.T) {
	fx := newFixture(t, 42, bytes.Repeat([]byte{0xAB}, 32))
	anchor := fx.writeAnchor(t, func(a map[string]any) {
		// Flip a byte in the signature.
		sig := append([]byte(nil), a["signature"].([]byte)...)
		sig[len(sig)-1] ^= 0x55
		a["signature"] = sig
	})
	if got := runMain(t, "--pubkey", fx.pubPath, "--anchor", anchor); got != exitBadSig {
		t.Errorf("exit = %d, want %d (bad sig)", got, exitBadSig)
	}
}

func TestRun_WrongPublicKey(t *testing.T) {
	fx := newFixture(t, 42, bytes.Repeat([]byte{0xAB}, 32))
	anchor := fx.writeAnchor(t, nil)
	// Write a different public key under the same pubPath.
	other, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	pubBytes, _ := x509.MarshalPKIXPublicKey(&other.PublicKey)
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubBytes})
	os.WriteFile(fx.pubPath, pubPEM, 0o600)

	if got := runMain(t, "--pubkey", fx.pubPath, "--anchor", anchor); got != exitBadSig {
		t.Errorf("exit = %d, want %d", got, exitBadSig)
	}
}

func TestRun_ReplayMatch(t *testing.T) {
	fx := newFixture(t, 42, bytes.Repeat([]byte{0xAB}, 32))
	anchor := fx.writeAnchor(t, nil)
	if got := runMain(t,
		"--pubkey", fx.pubPath,
		"--anchor", anchor,
		"--against-length", "42",
		"--against-tail-hash", hex.EncodeToString(bytes.Repeat([]byte{0xAB}, 32)),
	); got != exitOK {
		t.Errorf("matching replay exit = %d, want %d", got, exitOK)
	}
}

func TestRun_ReplayLengthMismatch(t *testing.T) {
	fx := newFixture(t, 42, bytes.Repeat([]byte{0xAB}, 32))
	anchor := fx.writeAnchor(t, nil)
	if got := runMain(t,
		"--pubkey", fx.pubPath,
		"--anchor", anchor,
		"--against-length", "999",
	); got != exitBadReplay {
		t.Errorf("replay length mismatch exit = %d, want %d", got, exitBadReplay)
	}
}

func TestRun_ReplayTailHashMismatch(t *testing.T) {
	fx := newFixture(t, 42, bytes.Repeat([]byte{0xAB}, 32))
	anchor := fx.writeAnchor(t, nil)
	if got := runMain(t,
		"--pubkey", fx.pubPath,
		"--anchor", anchor,
		"--against-tail-hash", hex.EncodeToString(bytes.Repeat([]byte{0xCC}, 32)),
	); got != exitBadReplay {
		t.Errorf("replay tail mismatch exit = %d, want %d", got, exitBadReplay)
	}
}

func TestRun_RejectsUnsupportedAlg(t *testing.T) {
	fx := newFixture(t, 42, bytes.Repeat([]byte{0xAB}, 32))
	anchor := fx.writeAnchor(t, func(a map[string]any) {
		a["signature_alg"] = "RSA-PSS"
	})
	if got := runMain(t, "--pubkey", fx.pubPath, "--anchor", anchor); got != exitBadInput {
		t.Errorf("unsupported alg exit = %d, want %d", got, exitBadInput)
	}
}

func TestRun_RejectsMissingSignature(t *testing.T) {
	fx := newFixture(t, 42, bytes.Repeat([]byte{0xAB}, 32))
	anchor := fx.writeAnchor(t, func(a map[string]any) {
		// JSON null/empty for signature.
		a["signed_payload"] = []byte{}
	})
	if got := runMain(t, "--pubkey", fx.pubPath, "--anchor", anchor); got != exitBadInput {
		t.Errorf("missing signed_payload exit = %d, want %d", got, exitBadInput)
	}
}

func TestRun_BadPubkey(t *testing.T) {
	fx := newFixture(t, 42, bytes.Repeat([]byte{0xAB}, 32))
	anchor := fx.writeAnchor(t, nil)
	bad := filepath.Join(fx.tmpDir, "bad.pem")
	os.WriteFile(bad, []byte("not a pem file"), 0o600)
	if got := runMain(t, "--pubkey", bad, "--anchor", anchor); got != exitBadInput {
		t.Errorf("bad pubkey exit = %d, want %d", got, exitBadInput)
	}
}
