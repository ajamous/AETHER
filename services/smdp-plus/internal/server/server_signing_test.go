package server

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ajamous/aether/pkg/hsmclient"
	smdpv1 "github.com/ajamous/aether/services/smdp-plus/api/v1"
	"github.com/ajamous/aether/services/smdp-plus/internal/identity"
	"github.com/ajamous/aether/services/smdp-plus/internal/session"
	"github.com/ajamous/aether/services/smdp-plus/internal/signing"
)

// TestInitiateAuthentication_SignatureVerifies wires up a minimal
// fake HSM broker that signs with a real ECDSA key, then drives the
// full pipeline:
//
//  1. SM-DP+ "starts up" by generating a DPauth keypair via the fake
//     broker (real ECDSA P-256 key on the broker side).
//  2. SM-DP+ self-signs an X.509 cert wrapping the public key.
//  3. LPA hits initiateAuthentication.
//  4. SM-DP+ builds ServerSigned1, asks the broker to sign the digest.
//  5. SM-DP+ returns ServerSigned1 + ServerSignature1 + ServerCertificate.
//  6. LPA-side verifier (this test) extracts the public key from the
//     cert, recomputes the digest over ServerSigned1, and verifies
//     the signature with stdlib ECDSA.
//
// If any link in the chain is wrong (DER encoding off, wrong digest,
// wrong key id, etc.), this test fails.
func TestInitiateAuthentication_SignatureVerifies(t *testing.T) {
	// --- Fake HSM broker ----------------------------------------------------
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	keyID := "test-DPauth-handle"
	pubPoint := elliptic.Marshal(elliptic.P256(), priv.PublicKey.X, priv.PublicKey.Y)

	brokerSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/v1/generate-key-pair"):
			json.NewEncoder(w).Encode(hsmclient.GenerateKeyPairResponse{
				Handle:    hsmclient.KeyHandle{ID: keyID, Label: "DPauth", Kind: hsmclient.KeyKindECDSA, Curve: hsmclient.CurveP256},
				PublicKey: pubPoint,
			})
		case strings.HasSuffix(r.URL.Path, "/v1/sign"):
			var req struct {
				KeyID  string `json:"key_id"`
				Digest []byte `json:"digest"`
			}
			json.NewDecoder(r.Body).Decode(&req)
			if req.KeyID != keyID {
				http.Error(w, "wrong key id", http.StatusNotFound)
				return
			}
			rInt, sInt, err := ecdsa.Sign(rand.Reader, priv, req.Digest)
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			der, _ := asn1.Marshal(struct{ R, S *big.Int }{R: rInt, S: sInt})
			json.NewEncoder(w).Encode(hsmclient.SignResponse{SignatureDER: der})
		default:
			http.NotFound(w, r)
		}
	}))
	defer brokerSrv.Close()

	// --- SM-DP+ startup -----------------------------------------------------
	hc := hsmclient.New(brokerSrv.URL)
	id, err := identity.EnsureLabIdentity(context.Background(), hc, "DPauth", "aether.local")
	if err != nil {
		t.Fatalf("identity: %v", err)
	}

	// Server with signing enabled.
	srv := httptest.NewServer(New(
		session.NewMemoryStore(time.Minute),
		Config{HSM: hc, Identity: id, Address: "aether.local"},
	).Routes())
	defer srv.Close()

	// --- LPA-side: drive initiateAuthentication and verify ------------------
	euiccChallenge := bytes.Repeat([]byte{0xAB}, 16)
	body, _ := json.Marshal(smdpv1.InitiateAuthenticationRequest{
		EUICCChallenge: euiccChallenge,
		SMDPAddress:    "aether.local",
	})
	resp, err := http.Post(srv.URL+"/gsma/rsp2/es9plus/initiateAuthentication", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var got smdpv1.InitiateAuthenticationResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(got.ServerSigned1) == 0 || len(got.ServerSignature1) == 0 || len(got.ServerCertificate) == 0 {
		t.Fatalf("response missing signed fields: %+v", got)
	}

	// Parse the ServerSigned1 we got back and check field correspondence.
	parsed, err := signing.UnmarshalServerSigned1(got.ServerSigned1)
	if err != nil {
		t.Fatalf("unmarshal serverSigned1: %v", err)
	}
	if !bytes.Equal(parsed.EUICCChallenge, euiccChallenge) {
		t.Fatalf("euiccChallenge mismatch")
	}
	if parsed.ServerAddress != "aether.local" {
		t.Fatalf("serverAddress = %q", parsed.ServerAddress)
	}
	if len(parsed.TransactionID) == 0 {
		t.Fatalf("empty transactionId in payload")
	}

	// Verify the signature with the cert's public key.
	cert, err := x509.ParseCertificate(got.ServerCertificate)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}
	pub, ok := cert.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		t.Fatalf("cert public key is not ECDSA: %T", cert.PublicKey)
	}
	digest := sha256.Sum256(got.ServerSigned1)
	var sigParsed struct{ R, S *big.Int }
	if _, err := asn1.Unmarshal(got.ServerSignature1, &sigParsed); err != nil {
		t.Fatalf("parse signature: %v", err)
	}
	if !ecdsa.Verify(pub, digest[:], sigParsed.R, sigParsed.S) {
		t.Fatal("ServerSignature1 did not verify against the cert's public key — pipeline is broken")
	}
}

// TestInitiateAuthentication_NoSigningWhenDisabled confirms the
// older "skeleton" behaviour: with no Config supplied, signed fields
// are nil rather than fabricated.
func TestInitiateAuthentication_NoSigningWhenDisabled(t *testing.T) {
	srv := httptest.NewServer(New(session.NewMemoryStore(time.Minute)).Routes())
	defer srv.Close()
	body, _ := json.Marshal(smdpv1.InitiateAuthenticationRequest{
		EUICCChallenge: bytes.Repeat([]byte{0xAB}, 16),
		SMDPAddress:    "aether.local",
	})
	resp, err := http.Post(srv.URL+"/gsma/rsp2/es9plus/initiateAuthentication", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var got smdpv1.InitiateAuthenticationResponse
	json.NewDecoder(resp.Body).Decode(&got)
	if len(got.ServerSigned1) != 0 || len(got.ServerSignature1) != 0 || len(got.ServerCertificate) != 0 {
		t.Fatalf("expected nil signed fields when signing disabled, got %+v", got)
	}
}
