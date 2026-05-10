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
		TransactionID:   []byte{0xDE, 0xAD, 0xBE, 0xEF},
		EUICCChallenge:  bytes.Repeat([]byte{0xAA}, 16),
		ServerAddress:   "aether.local",
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
	if !bytes.Equal(out.TransactionID, in.TransactionID) ||
		!bytes.Equal(out.EUICCChallenge, in.EUICCChallenge) ||
		out.ServerAddress != in.ServerAddress ||
		!bytes.Equal(out.ServerChallenge, in.ServerChallenge) {
		t.Fatalf("round-trip mismatch: %+v vs %+v", out, in)
	}
}

func TestServerSigned1_ValidationCatches(t *testing.T) {
	cases := map[string]ServerSigned1{
		"empty txid":    {EUICCChallenge: bytes.Repeat([]byte{1}, 16), ServerChallenge: bytes.Repeat([]byte{2}, 16), ServerAddress: "x"},
		"long txid":     {TransactionID: bytes.Repeat([]byte{1}, 17), EUICCChallenge: bytes.Repeat([]byte{1}, 16), ServerChallenge: bytes.Repeat([]byte{2}, 16), ServerAddress: "x"},
		"short euicc":   {TransactionID: []byte{1}, EUICCChallenge: bytes.Repeat([]byte{1}, 8), ServerChallenge: bytes.Repeat([]byte{2}, 16), ServerAddress: "x"},
		"empty address": {TransactionID: []byte{1}, EUICCChallenge: bytes.Repeat([]byte{1}, 16), ServerChallenge: bytes.Repeat([]byte{2}, 16)},
		"short server":  {TransactionID: []byte{1}, EUICCChallenge: bytes.Repeat([]byte{1}, 16), ServerChallenge: bytes.Repeat([]byte{2}, 8), ServerAddress: "x"},
	}
	for name, p := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := p.MarshalDER(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

// TestSignServerSigned1_VerifiesAgainstReturnedKey runs the full
// pipeline against a fake broker that signs with a real ECDSA key,
// then verifies the signature with stdlib ECDSA — proving the
// payload-build → hash → broker-sign → on-the-wire-DER chain is
// internally consistent.
func TestSignServerSigned1_VerifiesAgainstReturnedKey(t *testing.T) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/v1/sign") {
			http.NotFound(w, r)
			return
		}
		var req struct {
			KeyID  string `json:"key_id"`
			Digest []byte `json:"digest"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		rInt, sInt, err := ecdsa.Sign(rand.Reader, priv, req.Digest)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		der, _ := asn1.Marshal(struct{ R, S *big.Int }{R: rInt, S: sInt})
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(hsmclient.SignResponse{SignatureDER: der})
	}))
	defer srv.Close()

	hc := hsmclient.New(srv.URL)

	payload := ServerSigned1{
		TransactionID:   []byte{0x00, 0x11, 0x22, 0x33},
		EUICCChallenge:  bytes.Repeat([]byte{0xAA}, 16),
		ServerAddress:   "aether.local",
		ServerChallenge: bytes.Repeat([]byte{0xBB}, 16),
	}
	signedDER, sig, err := SignServerSigned1(context.Background(), hc, "any-key-id", payload)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	digest := sha256.Sum256(signedDER)
	var parsed struct{ R, S *big.Int }
	if _, err := asn1.Unmarshal(sig, &parsed); err != nil {
		t.Fatalf("parse signature: %v", err)
	}
	if !ecdsa.Verify(&priv.PublicKey, digest[:], parsed.R, parsed.S) {
		t.Fatal("signature did not verify against the test broker's key")
	}
}
