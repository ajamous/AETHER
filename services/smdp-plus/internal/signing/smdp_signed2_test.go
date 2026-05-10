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

func TestSmdpSigned2_RoundTrip_NoOtpk(t *testing.T) {
	in := SmdpSigned2{
		TransactionID:  []byte{0x01, 0x02, 0x03, 0x04},
		CCRequiredFlag: false,
		// BPPEuiccOtpk omitted — the field is OPTIONAL on the wire.
	}
	der, err := in.MarshalDER()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out, err := UnmarshalSmdpSigned2(der)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !bytes.Equal(out.TransactionID, in.TransactionID) {
		t.Errorf("tid mismatch")
	}
	if out.CCRequiredFlag != in.CCRequiredFlag {
		t.Errorf("ccRequiredFlag = %v", out.CCRequiredFlag)
	}
	if len(out.BPPEuiccOtpk) != 0 {
		t.Errorf("BPPEuiccOtpk should be empty, got %d bytes", len(out.BPPEuiccOtpk))
	}
}

func TestSmdpSigned2_RoundTrip_WithUncompressedOtpk(t *testing.T) {
	// Uncompressed P-256 point: 0x04 || X(32) || Y(32) = 65 bytes.
	otpk := append([]byte{0x04}, bytes.Repeat([]byte{0xAB}, 64)...)
	in := SmdpSigned2{
		TransactionID:  bytes.Repeat([]byte{0xCC}, 16),
		CCRequiredFlag: true,
		BPPEuiccOtpk:   otpk,
	}
	der, err := in.MarshalDER()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out, err := UnmarshalSmdpSigned2(der)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !bytes.Equal(out.BPPEuiccOtpk, otpk) {
		t.Error("BPPEuiccOtpk mismatch after round-trip")
	}
	if out.CCRequiredFlag != true {
		t.Error("ccRequiredFlag should be true")
	}
}

func TestSmdpSigned2_RoundTrip_WithCompressedOtpk(t *testing.T) {
	// Compressed P-256 point: 0x02 || X(32) = 33 bytes.
	otpk := append([]byte{0x03}, bytes.Repeat([]byte{0xDE}, 32)...)
	in := SmdpSigned2{
		TransactionID:  []byte{0x01},
		CCRequiredFlag: false,
		BPPEuiccOtpk:   otpk,
	}
	der, err := in.MarshalDER()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out, _ := UnmarshalSmdpSigned2(der)
	if !bytes.Equal(out.BPPEuiccOtpk, otpk) {
		t.Error("compressed otpk did not round-trip")
	}
}

func TestSmdpSigned2_ValidationCatches(t *testing.T) {
	cases := []struct {
		name string
		in   SmdpSigned2
		want string
	}{
		{
			"empty tid",
			SmdpSigned2{},
			"transactionId",
		},
		{
			"tid too long",
			SmdpSigned2{TransactionID: bytes.Repeat([]byte{0}, 17)},
			"transactionId",
		},
		{
			"otpk wrong length",
			SmdpSigned2{
				TransactionID: []byte{0x01},
				BPPEuiccOtpk:  []byte{0x04, 0xAA, 0xBB}, // 3 bytes — neither 33 nor 65
			},
			"bppEuiccOtpk length 3",
		},
		{
			"compressed otpk bad first byte",
			SmdpSigned2{
				TransactionID: []byte{0x01},
				BPPEuiccOtpk:  append([]byte{0x05}, bytes.Repeat([]byte{0}, 32)...), // 0x05 not 0x02/0x03
			},
			"compressed-point first byte",
		},
		{
			"uncompressed otpk bad first byte",
			SmdpSigned2{
				TransactionID: []byte{0x01},
				BPPEuiccOtpk:  append([]byte{0x05}, bytes.Repeat([]byte{0}, 64)...), // 0x05 not 0x04
			},
			"uncompressed-point first byte",
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

func TestUnmarshalSmdpSigned2_RejectsTrailingBytes(t *testing.T) {
	in := SmdpSigned2{
		TransactionID: []byte{0x01},
	}
	der, _ := in.MarshalDER()
	der = append(der, 0xFF)
	if _, err := UnmarshalSmdpSigned2(der); err == nil {
		t.Fatal("expected trailing-bytes error")
	}
}

// TestSignSmdpSigned2_VerifiesEndToEnd stands up a fake HSM
// broker, signs a SmdpSigned2 against a P-256 key, and verifies
// the signature against the broker's public key. Mirrors the
// equivalent ServerSigned1 / smds-anchor tests so the
// SAS-SM-relevant signing path stays covered end-to-end.
func TestSignSmdpSigned2_VerifiesEndToEnd(t *testing.T) {
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
		if req.KeyID != "smdp-dppb-key" {
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
	payload := SmdpSigned2{
		TransactionID:  []byte{0x10, 0x20, 0x30, 0x40},
		CCRequiredFlag: true,
		BPPEuiccOtpk:   append([]byte{0x04}, bytes.Repeat([]byte{0xAB}, 64)...),
	}
	signed, sig, err := SignSmdpSigned2(context.Background(), hc, "smdp-dppb-key", payload)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	digest := sha256.Sum256(signed)
	var ecdsaSig struct{ R, S *big.Int }
	if _, err := asn1.Unmarshal(sig, &ecdsaSig); err != nil {
		t.Fatalf("sig unmarshal: %v", err)
	}
	if !ecdsa.Verify(&priv.PublicKey, digest[:], ecdsaSig.R, ecdsaSig.S) {
		t.Fatal("ECDSA verify failed against the broker's public key")
	}

	// Round-trip the signed payload through Unmarshal so callers
	// can confirm the on-the-wire bytes decode to fields matching
	// what they intended to sign.
	out, err := UnmarshalSmdpSigned2(signed)
	if err != nil {
		t.Fatalf("unmarshal signed: %v", err)
	}
	if !bytes.Equal(out.TransactionID, payload.TransactionID) {
		t.Error("tid drifted between sign and verify")
	}
	if !bytes.Equal(out.BPPEuiccOtpk, payload.BPPEuiccOtpk) {
		t.Error("BPPEuiccOtpk drifted between sign and verify")
	}
}
