package signing

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

	"github.com/ajamous/aether/pkg/hsmclient"
)

func TestServerSigned1_RoundTrip(t *testing.T) {
	in := ServerSigned1{
		TransactionID:   []byte{0x01, 0x02, 0x03, 0x04},
		EUICCChallenge:  bytes.Repeat([]byte{0xAA}, 16),
		ServerAddress:   "smds.aether.local",
		ServerChallenge: bytes.Repeat([]byte{0xBB}, 16),
	}
	der, err := in.MarshalDER()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out, err := UnmarshalServerSigned1(der)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !bytes.Equal(out.TransactionID, in.TransactionID) {
		t.Errorf("tid mismatch")
	}
	if !bytes.Equal(out.EUICCChallenge, in.EUICCChallenge) {
		t.Errorf("euiccChallenge mismatch")
	}
	if out.ServerAddress != in.ServerAddress {
		t.Errorf("serverAddress mismatch: %q vs %q", out.ServerAddress, in.ServerAddress)
	}
	if !bytes.Equal(out.ServerChallenge, in.ServerChallenge) {
		t.Errorf("serverChallenge mismatch")
	}
}

func TestServerSigned1_ValidationCatches(t *testing.T) {
	cases := []struct {
		name string
		in   ServerSigned1
		want string
	}{
		{
			"empty tid",
			ServerSigned1{
				EUICCChallenge:  bytes.Repeat([]byte{0}, 16),
				ServerAddress:   "x",
				ServerChallenge: bytes.Repeat([]byte{0}, 16),
			},
			"transactionId",
		},
		{
			"tid too long",
			ServerSigned1{
				TransactionID:   bytes.Repeat([]byte{0}, 17),
				EUICCChallenge:  bytes.Repeat([]byte{0}, 16),
				ServerAddress:   "x",
				ServerChallenge: bytes.Repeat([]byte{0}, 16),
			},
			"transactionId",
		},
		{
			"bad euicc challenge",
			ServerSigned1{
				TransactionID:   []byte{0x01},
				EUICCChallenge:  []byte{0x01, 0x02},
				ServerAddress:   "x",
				ServerChallenge: bytes.Repeat([]byte{0}, 16),
			},
			"euiccChallenge",
		},
		{
			"bad server challenge",
			ServerSigned1{
				TransactionID:   []byte{0x01},
				EUICCChallenge:  bytes.Repeat([]byte{0}, 16),
				ServerAddress:   "x",
				ServerChallenge: []byte{0x01, 0x02},
			},
			"serverChallenge",
		},
		{
			"empty serverAddress",
			ServerSigned1{
				TransactionID:   []byte{0x01},
				EUICCChallenge:  bytes.Repeat([]byte{0}, 16),
				ServerAddress:   "",
				ServerChallenge: bytes.Repeat([]byte{0}, 16),
			},
			"serverAddress",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := c.in.MarshalDER()
			if err == nil {
				t.Fatalf("expected error containing %q", c.want)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %q did not mention %q", err.Error(), c.want)
			}
		})
	}
}

// TestSignServerSigned1_VerifiesEndToEnd stands up a fake HSM broker
// HTTP server that signs with a generated P-256 key, runs
// SignServerSigned1, then verifies the returned ECDSA-SHA-256
// signature against the broker's public key. Mirrors the
// smdp-plus equivalent.
func TestSignServerSigned1_VerifiesEndToEnd(t *testing.T) {
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
		rr, ss, err := ecdsa.Sign(rand.Reader, priv, req.Digest)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		der, _ := asn1.Marshal(struct{ R, S *big.Int }{rr, ss})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"signature_der": der,
		})
	}))
	defer srv.Close()

	hc := hsmclient.New(srv.URL)
	payload := ServerSigned1{
		TransactionID:   []byte{0x10, 0x20, 0x30, 0x40},
		EUICCChallenge:  bytes.Repeat([]byte{0xCC}, 16),
		ServerAddress:   "smds.aether.local",
		ServerChallenge: bytes.Repeat([]byte{0xDD}, 16),
	}
	signed, sig, err := SignServerSigned1(context.Background(), hc, "smds-auth-key", payload)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	digest := sha256.Sum256(signed)
	var ecdsaSig struct{ R, S *big.Int }
	if _, err := asn1.Unmarshal(sig, &ecdsaSig); err != nil {
		t.Fatalf("sig unmarshal: %v", err)
	}
	if !ecdsa.Verify(&priv.PublicKey, digest[:], ecdsaSig.R, ecdsaSig.S) {
		t.Fatal("ECDSA Verify failed against the public key the broker used to sign")
	}
}

func TestHexTransactionID(t *testing.T) {
	if got := HexTransactionID([]byte{0xab, 0xcd, 0xef}); got != "abcdef" {
		t.Errorf("got %q", got)
	}
}
