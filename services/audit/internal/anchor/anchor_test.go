package anchor

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/asn1"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ajamous/aether/pkg/hsmclient"
)

func TestAnchor_RoundTrip(t *testing.T) {
	in := Anchor{
		Timestamp: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Length:    1234,
		TailHash:  bytes.Repeat([]byte{0xAB}, 32),
	}
	der, err := in.MarshalDER()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out, err := UnmarshalAnchor(der)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !out.Timestamp.Equal(in.Timestamp) {
		t.Errorf("timestamp mismatch: got %s, want %s", out.Timestamp, in.Timestamp)
	}
	if out.Length != in.Length {
		t.Errorf("length: got %d, want %d", out.Length, in.Length)
	}
	if !bytes.Equal(out.TailHash, in.TailHash) {
		t.Errorf("tail hash mismatch")
	}
}

func TestAnchor_Validation(t *testing.T) {
	cases := []struct {
		name    string
		in      Anchor
		wantErr string
	}{
		{
			"empty tail hash",
			Anchor{Timestamp: time.Now(), Length: 1, TailHash: nil},
			"tail hash",
		},
		{
			"short tail hash",
			Anchor{Timestamp: time.Now(), Length: 1, TailHash: []byte{0x01, 0x02}},
			"tail hash",
		},
		{
			"negative length",
			Anchor{Timestamp: time.Now(), Length: -1, TailHash: bytes.Repeat([]byte{0}, 32)},
			"length",
		},
		{
			"zero timestamp",
			Anchor{Length: 1, TailHash: bytes.Repeat([]byte{0}, 32)},
			"timestamp",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := c.in.MarshalDER()
			if err == nil {
				t.Fatalf("expected error containing %q", c.wantErr)
			}
			if !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("error %q did not mention %q", err.Error(), c.wantErr)
			}
		})
	}
}

// TestSign_VerifiesEndToEnd stands up a fake HSM broker that signs
// with a generated P-256 key, asks Sign to produce a signed
// anchor, and verifies the returned ECDSA signature against the
// broker's public key.
func TestSign_VerifiesEndToEnd(t *testing.T) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/sign") {
			http.NotFound(w, r)
			return
		}
		var req struct {
			KeyID  string `json:"key_id"`
			Digest []byte `json:"digest"`
			Hash   string `json:"hash"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		if req.KeyID != "audit-anchor-key" {
			http.Error(w, "wrong key id", 400)
			return
		}
		rr, ss, err := ecdsa.Sign(rand.Reader, priv, req.Digest)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		der, _ := asn1.Marshal(struct{ R, S *big.Int }{rr, ss})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"signature_der": der})
	}))
	defer srv.Close()

	hc := hsmclient.New(srv.URL)
	a := Anchor{
		Timestamp: time.Now().UTC().Truncate(time.Second),
		Length:    42,
		TailHash:  bytes.Repeat([]byte{0xCC}, 32),
	}
	signed, sig, err := Sign(context.Background(), hc, "audit-anchor-key", a)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	// Round-trip and verify.
	parsed, err := UnmarshalAnchor(signed)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.Length != 42 {
		t.Errorf("parsed length = %d", parsed.Length)
	}

	digest := sha256.Sum256(signed)
	var ecdsaSig struct{ R, S *big.Int }
	if _, err := asn1.Unmarshal(sig, &ecdsaSig); err != nil {
		t.Fatalf("sig unmarshal: %v", err)
	}
	if !ecdsa.Verify(&priv.PublicKey, digest[:], ecdsaSig.R, ecdsaSig.S) {
		t.Fatal("ECDSA verify failed against the broker's public key")
	}
}

func TestHexHash(t *testing.T) {
	if got := HexHash([]byte{0xab, 0xcd}); got != "abcd" {
		t.Errorf("got %q", got)
	}
}
